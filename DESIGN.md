# DCM — Declarative Cloud Manager

## What we want to combine

| Capability | Inspiration |
|---|---|
| Infrastructure as Code with dependency graph | Terraform |
| Configuration management / orchestration | Ansible |
| Kubernetes-native composition of resources | Kro / Crossplane |
| GitOps reconciliation loop | ArgoCD / Flux |
| Policy-driven provider selection | OPA / Kyverno |

## Key Architectural Decisions

### 1. Declarative Model — How do users define applications?

**Option A: Custom YAML/HCL DSL**
- Like Terraform's HCL or Kubernetes manifests
- Pros: familiar, tooling-friendly, easy to template
- Cons: another DSL to learn, limited expressiveness

**Option B: Code-based SDK (CDK-style)**
- Like Pulumi/AWS CDK — users write Go/TypeScript/Python
- Pros: full programming language power, type safety, testable
- Cons: harder to lint/validate statically, steeper learning curve

**Option C: Hybrid — YAML with embedded expressions**
- Like Helm/Kustomize but richer
- Pros: simple cases stay simple, complex cases are possible
- Cons: can get messy

**Recommendation:** Start with **structured YAML specs** (like Kubernetes CRDs) with a clear schema. Add a Go SDK later for power users. YAML is the lingua franca of platform engineering and integrates naturally with GitOps.

---

### 2. Execution Model — How does it deploy?

**Option A: Client-side CLI (like Terraform)**
- CLI reads specs, builds a dependency DAG, applies changes
- Pros: simple, no server needed, easy to debug
- Cons: no continuous reconciliation, state drift

**Option B: Controller/Operator (like Crossplane/Kro)**
- A server watches desired state and reconciles continuously
- Pros: self-healing, GitOps-native, handles drift
- Cons: more complex to build and operate

**Option C: Hybrid — CLI for dev, Controller for prod**
- Best of both worlds

**Recommendation:** **Hybrid**. Build a core engine library that can run in both CLI mode (for local dev/testing) and controller mode (for production GitOps). The engine does DAG resolution, diffing, and provider dispatch — the shell (CLI vs controller) just determines how it's triggered.

---

### 3. Provider Abstraction — How do you support multiple clouds?

**Option A: Provider plugins (Terraform model)**
- Each provider is a separate binary/plugin
- Pros: extensible, community-driven
- Cons: complex plugin protocol, version management

**Option B: Built-in adapters with a common interface**
- Provider logic lives in the core codebase behind an interface
- Pros: simpler to start, consistent behavior
- Cons: harder to scale to many providers

**Option C: CRD + Controller per provider (Crossplane model)**
- Each provider is a separate controller managing its own CRDs
- Pros: Kubernetes-native, independently deployable
- Cons: heavy, requires K8s

**Recommendation:** Start with **built-in adapters behind a `Provider` interface** in Go. Define a clean contract:

```go
type Provider interface {
    Name() string
    Capabilities() []ResourceType
    Plan(desired, current Resource) (Diff, error)
    Apply(diff Diff) (Resource, error)
    Destroy(resource Resource) error
    Status(resource Resource) (ResourceStatus, error)
}
```

Later, you can extract these into plugins (via gRPC like Terraform or WASM for sandboxing).

---

### 4. Dependency Graph & Composition

This is the core differentiator. An application is a **DAG of resources** with typed inputs/outputs:

```yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: my-web-app
spec:
  components:
    - name: database
      type: postgres
      properties:
        version: "16"
        storage: 50Gi

    - name: cache
      type: redis
      properties:
        version: "7"

    - name: backend
      type: container
      dependsOn: [database, cache]
      properties:
        image: myapp/backend:latest
        env:
          DATABASE_URL: "{{ components.database.outputs.connectionString }}"
          REDIS_URL: "{{ components.cache.outputs.endpoint }}"

    - name: frontend
      type: static-site
      dependsOn: [backend]
      properties:
        source: ./frontend
        apiEndpoint: "{{ components.backend.outputs.url }}"
```

The engine resolves the DAG, topologically sorts, and applies in order (with parallelism for independent nodes).

---

### 5. Policy-Driven Provider Selection

Policies determine **where** things run:

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: production-placement
spec:
  rules:
    - match:
        labels:
          env: production
          data-residency: eu
      providers:
        preferred: [aws-eu-west-1, gcp-europe-west1]
        forbidden: [aws-us-east-1]

    - match:
        type: postgres
        labels:
          tier: critical
      providers:
        preferred: [aws]  # RDS for critical DBs
      properties:
        multiAz: true
        backupRetention: 30d

    - match:
        type: container
        labels:
          cost: optimize
      providers:
        strategy: cheapest  # auto-select cheapest provider
```

For the policy engine, the recommendation is to embed **CEL (Common Expression Language)** — it's what Kubernetes uses for validation, it's fast, sandboxed, and well-understood in the platform engineering space.

---

### 6. GitOps

Store application specs in Git. The controller watches the repo (or receives webhooks) and reconciles:

```
repo/
├── apps/
│   ├── web-app/
│   │   ├── app.yaml
│   │   └── overrides/
│   │       ├── staging.yaml
│   │       └── production.yaml
│   └── data-pipeline/
│       └── app.yaml
├── policies/
│   ├── placement.yaml
│   └── cost.yaml
└── providers/
    ├── aws.yaml
    └── gcp.yaml
```

---

## Recommended Tech Stack

| Component | Technology | Why |
|---|---|---|
| Language | **Go** | Standard for infra tools, great concurrency, strong K8s ecosystem |
| Config format | **YAML + CEL expressions** | Familiar, powerful enough |
| DAG engine | Custom (Go) | Not complex, ~500 lines for a solid DAG executor |
| Policy engine | **CEL** | Fast, safe, Kubernetes-aligned |
| State storage | **Local file + S3/GCS + CRDs** | Pluggable backends |
| Provider protocol | Go interface (later gRPC) | Start simple |
| GitOps | **Git polling + webhooks** | Or integrate with Flux/ArgoCD |
| CLI framework | **Cobra** | Industry standard for Go CLIs |

## Suggested MVP Scope

For a first iteration, focus on:

1. **Core spec format** — Application YAML with components and dependencies
2. **DAG engine** — Resolve, plan, apply with topological ordering
3. **2 providers** — e.g., AWS + a local/Docker provider for testing
4. **CLI** — `dcm plan`, `dcm apply`, `dcm destroy`, `dcm status`
5. **Basic policy** — Provider selection based on labels
6. **State file** — Track what's deployed

Defer to v2: GitOps controller, plugin system, UI, drift detection.
