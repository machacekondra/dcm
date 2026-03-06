# DCM — Declarative Cloud Manager

DCM is a platform engineering tool that lets you define multi-component applications in YAML and deploy them to any cloud provider. It combines infrastructure-as-code, dependency management, and policy-driven provider selection into a single workflow.

## What it does

- **Define applications as components** — databases, containers, caches, and static sites with explicit dependency relationships
- **Deploy to any provider** — a provider abstraction layer routes each component to the right infrastructure (Kubernetes, AWS, mock, etc.)
- **Use policies to control placement** — rules based on labels, component types, or CEL expressions determine which providers are used, without modifying application definitions
- **Plan before you apply** — preview exactly what will be created, updated, or destroyed before making changes
- **Track deployment state** — full audit trail of every deployment action

## Use cases

**Multi-cloud deployment** — deploy the same application to different providers based on environment. Use mock providers for development, Kubernetes for staging, and managed cloud services for production — all from the same application definition.

**Policy-driven infrastructure** — platform teams define policies like "production databases must use AWS" or "EU workloads must not use US regions". Application teams deploy without worrying about provider details.

**Dependency-aware orchestration** — define that your backend depends on a database and cache, and your frontend depends on the backend. DCM deploys them in the right order and passes connection details between components automatically.

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

### Add policies

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
│  apply      │     │  - Executor │     │  - AWS ...  │
│  destroy    │     │             │     │             │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    ┌──────▼──────┐
                    │  Policy     │
                    │  Engine     │
                    │  (CEL)      │
                    └─────────────┘
```

**Engine** — builds a dependency DAG from application components, computes a plan (create/update/delete diffs), and executes it in topological order.

**Providers** — implement a common interface (`Plan`, `Apply`, `Destroy`, `Status`) for each infrastructure backend. The mock provider works out of the box for testing; the Kubernetes provider creates Deployments and Services.

**Policy engine** — evaluates rules against components using type matching, label matching, and CEL expressions. Determines which provider handles each component and can inject additional properties.

## Documentation

- [Applications](doc/app.md) — how to define and manage applications
- [Policy engine](doc/policy.md) — how policies control provider selection
- [API reference](doc/api.md) — REST API endpoints and usage

## Project structure

```
├── cmd/                  # CLI commands (plan, apply, destroy, status, serve, policy)
├── pkg/
│   ├── types/            # Core types (Application, Policy, Provider, Resource)
│   ├── engine/           # DAG builder, planner, executor
│   ├── policy/           # CEL-based policy evaluator
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
