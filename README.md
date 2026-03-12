# DCM — Declarative Cloud Manager

DCM is a platform engineering tool that lets you define multi-component applications in YAML and deploy them to any cloud provider. It combines infrastructure-as-code, dependency management, and policy-driven provider selection into a single workflow.

## What it does

- **Define applications as components** — databases, containers, caches, and static sites with explicit dependency relationships
- **Deploy to any provider** — a provider abstraction layer routes each component to the right infrastructure (Kubernetes, AWS, mock, etc.)
- **Control placement with rules** — CEL expressions, labels, and type matching determine which providers handle each component, without modifying application definitions
- **Enforce guardrails with OPA** — Rego rules block deployments that violate organizational standards (storage minimums, replica requirements, cost limits) before any resources are created
- **Plan before you apply** — preview exactly what will be created, updated, or destroyed before making changes
- **Track deployment state** — full audit trail of every deployment action

## Use cases

**Multi-cloud deployment** — deploy the same application to different providers based on environment. Use mock providers for development, Kubernetes for staging, and managed cloud services for production — all from the same application definition.

**Placement rules** — platform teams define rules like "production databases must use AWS" or "EU workloads must not use US regions". Application teams deploy without worrying about provider details.

**Dependency-aware orchestration** — define that your backend depends on a database and cache, and your frontend depends on the backend. DCM deploys them in the right order and passes connection details between components automatically.

**Guardrails** — enforce organizational standards like "production databases must have at least 10Gi storage" or "containers in production must specify replicas". OPA/Rego guardrails are evaluated after planning but before any resources are touched, giving teams a safety net without slowing them down.

**Standardized application templates** — define reusable application patterns (web app with database, microservice with cache) that teams can deploy consistently across environments.

## Quick start

### Define an application

```yaml
# app.yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: my-web-app
  labels:
    env: production
spec:
  components:
    - name: database
      type: postgres
      properties:
        version: "16"
        storage: 50Gi

    - name: backend
      type: container
      dependsOn: [database]
      properties:
        image: myapp/backend:latest
        replicas: 3
        env:
          DATABASE_URL: "{{ components.database.outputs.connectionString }}"
```

### Plan and deploy

```bash
# Build the CLI
make build

# Preview changes
./dcm plan -f app.yaml

# Deploy
./dcm apply -f app.yaml

# Check status
./dcm status -f app.yaml

# Tear down
./dcm destroy -f app.yaml
```

### Add placement rules

```yaml
# policies/placement.yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: production-rules
spec:
  rules:
    - name: no-mock-in-prod
      priority: 200
      match:
        expression: 'app.labels.env == "production"'
      providers:
        forbidden: [mock]

    - name: databases-on-aws
      priority: 100
      match:
        type: postgres
      providers:
        preferred: [aws]
```

```bash
./dcm plan -f app.yaml -p policies/
./dcm apply -f app.yaml -p policies/
```

### Add guardrails

Place Rego files in `data/policies/` to enforce infrastructure constraints:

```rego
# data/policies/production.rego
package dcm.compliance

deny contains msg if {
    input.component.type == "postgres"
    input.environment.labels.env == "prod"
    storage := input.component.properties.storage
    not startswith(storage, "10")
    not startswith(storage, "20")
    not startswith(storage, "50")
    not startswith(storage, "100")
    msg := sprintf("postgres %q in prod requires at least 10Gi storage, got %s",
                   [input.component.name, storage])
}

deny contains msg if {
    input.component.type == "container"
    input.environment.labels.env == "prod"
    not input.component.properties.replicas
    msg := sprintf("container %q in prod must specify replicas",
                   [input.component.name])
}
```

Guardrails are loaded automatically on server startup and evaluated before every deployment. You can also check compliance without deploying:

```bash
curl -X POST http://localhost:8080/api/v1/compliance/check \
  -H 'Content-Type: application/json' \
  -d '{"application": "my-web-app"}'
```

### Run the API server and UI

```bash
# Start the API server
make serve

# In another terminal, start the UI dev server
make ui
```

The API is available at `http://localhost:8080` and the UI at `http://localhost:5173`.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   CLI / UI  │────▶│  Engine     │────▶│  Providers  │
│             │     │  - DAG      │     │  - Mock     │
│  plan       │     │  - Planner  │     │  - K8s      │
│  apply      │     │  - Executor │     │  - Postgres │
│  destroy    │     │             │     │  - AWS ...  │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                ┌──────────┼──────────┐
                │                     │
         ┌──────▼──────┐      ┌──────▼──────┐
         │  Placement  │      │  Guardrails │
         │  Rules      │      │             │
         │  (CEL)      │      │  (OPA/Rego) │
         └─────────────┘      └─────────────┘
```

**Engine** — builds a dependency DAG from application components, computes a plan (create/update/delete diffs), and executes it in topological order.

**Providers** — implement a common interface (`Plan`, `Apply`, `Destroy`, `Status`) for each infrastructure backend. The mock provider works out of the box for testing; the Kubernetes provider creates Deployments and Services.

**Placement rules** — evaluates CEL expressions, labels, and type matching to determine which provider handles each component. Can also inject properties into components.

**Guardrails** — evaluates OPA/Rego rules against the computed plan before resources are created. Blocks deployments that violate organizational constraints like storage minimums, replica requirements, and cost limits.

## Documentation

- [Applications](doc/app.md) — how to define and manage applications
- [Placement rules](doc/policy.md) — how CEL rules control provider selection
- [Guardrails](doc/compliance.md) — how OPA/Rego rules enforce infrastructure standards
- [Health checks](doc/health-checks.md) — active environment health probing and automatic failover
- [API reference](doc/api.md) — REST API endpoints and usage

## Project structure

```
├── cmd/                  # CLI commands (plan, apply, destroy, status, serve, policy)
├── pkg/
│   ├── types/            # Core types (Application, Policy, Provider, Resource)
│   ├── engine/           # DAG builder, planner, executor
│   ├── policy/           # Placement rules (CEL-based)
│   ├── compliance/       # Guardrails (OPA/Rego)
│   ├── provider/         # Provider implementations (mock, kubernetes)
│   ├── loader/           # YAML file loading
│   ├── store/            # SQLite persistence for API mode
│   ├── state/            # File-based state for CLI mode
│   └── api/              # REST API server
├── ui/                   # React + PatternFly web interface
├── examples/             # Example applications and policies
└── doc/                  # Documentation
```

## Requirements

- Go 1.25+
- Node.js 18+ (for UI development)
