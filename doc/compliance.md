# Compliance Engine

DCM's compliance engine uses [Open Policy Agent (OPA)](https://www.openpolicyagent.org/) and [Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) to enforce infrastructure policies before deployments are applied. While the [policy engine](policy.md) controls **provider selection** (which provider handles a component), the compliance engine controls **what is allowed** (whether a deployment can proceed at all).

## Concepts

### How it fits in the deployment flow

```
Create Deployment
  │
  ▼
Planning ──── scheduler + policy engine select providers
  │
  ▼
Compliance Check ──── OPA evaluates Rego policies against the plan
  │
  ├── violations found → deployment fails with details
  │
  └── no violations → continues
  │
  ▼
Applying ──── providers create/update resources
  │
  ▼
Ready
```

The compliance check runs **after** the plan is computed but **before** any resources are created or modified. This means policies can inspect the full context: which component is being deployed, what properties it has, which environment it targets, and what action is being taken.

### Policy vs. Compliance

| Concern | Engine | Language | Effect |
|---------|--------|----------|--------|
| Provider selection | Policy (CEL) | CEL expressions | Choose/restrict which provider handles a component |
| Infrastructure rules | Compliance (OPA) | Rego | Block deployments that violate organizational rules |

Use **policies** for placement decisions ("databases go to AWS", "no mock provider in production"). Use **compliance** for organizational constraints ("production databases need 10Gi+ storage", "containers in production must specify replicas", "no environment can exceed $1/hr").

## Writing Rego policies

Compliance policies are Rego files that define `deny` rules in the `dcm.compliance` package. Each rule that evaluates to true produces a violation message that blocks the deployment.

### Package and rule structure

All policies must use the `dcm.compliance` package and define partial set rules using Rego v1 syntax:

```rego
package dcm.compliance

deny contains msg if {
    # conditions...
    msg := "violation message"
}
```

The `deny` set collects all violation messages. If the set is empty after evaluation, the deployment proceeds. If it contains any messages, the deployment fails and the messages are reported.

### Input schema

Each plan step is evaluated independently. The input object has this structure:

```json
{
  "component": {
    "name": "database",
    "type": "postgres",
    "labels": {"tier": "critical"},
    "properties": {"version": "16", "storage": "50Gi"},
    "requires": ["persistence"]
  },
  "environment": {
    "name": "prod-cluster",
    "provider": "kubernetes",
    "labels": {"env": "prod", "region": "eu-west-1"},
    "capabilities": ["container", "postgres"],
    "cost": {
      "tier": "premium",
      "hourlyRate": 0.50
    }
  },
  "action": "create",
  "application": {
    "name": "my-web-app",
    "labels": {"env": "production", "team": "platform"}
  }
}
```

#### Input fields

| Path | Type | Description |
|------|------|-------------|
| `input.component.name` | string | Component name |
| `input.component.type` | string | Resource type (`container`, `postgres`, `redis`, etc.) |
| `input.component.labels` | object | Component labels |
| `input.component.properties` | object | Component properties |
| `input.component.requires` | array | Required capabilities |
| `input.environment.name` | string | Target environment name |
| `input.environment.provider` | string | Provider type (`kubernetes`, `aws`, etc.) |
| `input.environment.labels` | object | Environment labels |
| `input.environment.capabilities` | array | Supported resource types |
| `input.environment.cost` | object | Cost metadata (may be null) |
| `input.environment.cost.tier` | string | Cost tier |
| `input.environment.cost.hourlyRate` | number | Hourly rate in dollars |
| `input.action` | string | Plan action: `create`, `update`, `delete`, or `none` |
| `input.application.name` | string | Application name |
| `input.application.labels` | object | Application labels |

## Examples

### Production storage requirements

Require production PostgreSQL databases to have at least 10Gi storage:

```rego
package dcm.compliance

deny contains msg if {
    input.component.type == "postgres"
    input.environment.labels.env == "prod"
    storage := input.component.properties.storage
    not startswith(storage, "10")
    not startswith(storage, "20")
    not startswith(storage, "50")
    not startswith(storage, "100")
    msg := sprintf(
        "postgres %q in prod requires at least 10Gi storage, got %s",
        [input.component.name, storage],
    )
}
```

### Require replicas in production

Ensure containers deployed to production specify a replica count:

```rego
package dcm.compliance

deny contains msg if {
    input.component.type == "container"
    input.environment.labels.env == "prod"
    not input.component.properties.replicas
    msg := sprintf(
        "container %q in prod must specify replicas",
        [input.component.name],
    )
}
```

### Cost limits

Prevent deployments to environments that exceed a cost threshold:

```rego
package dcm.compliance

deny contains msg if {
    input.environment.cost.hourlyRate > 1.0
    msg := sprintf(
        "environment %q exceeds cost limit ($%.2f/hr)",
        [input.environment.name, input.environment.cost.hourlyRate],
    )
}
```

### Restrict container images

Only allow images from an approved registry:

```rego
package dcm.compliance

deny contains msg if {
    input.component.type == "container"
    image := input.component.properties.image
    not startswith(image, "registry.example.com/")
    msg := sprintf(
        "container %q uses unapproved image %q — must use registry.example.com",
        [input.component.name, image],
    )
}
```

### Environment-specific rules

Block certain resource types from dev environments:

```rego
package dcm.compliance

deny contains msg if {
    input.component.type == "vm"
    input.environment.labels.env == "dev"
    msg := sprintf(
        "VMs are not allowed in dev environment %q — use containers instead",
        [input.environment.name],
    )
}
```

### Multiple rules in one file

You can define multiple `deny` rules in a single file. All rules are evaluated and all violations are collected:

```rego
package dcm.compliance

# Prod databases need sufficient storage.
deny contains msg if {
    input.component.type == "postgres"
    input.environment.labels.env == "prod"
    storage := input.component.properties.storage
    not startswith(storage, "10")
    not startswith(storage, "20")
    not startswith(storage, "50")
    not startswith(storage, "100")
    msg := sprintf("postgres %q: insufficient storage %s for prod", [input.component.name, storage])
}

# Prod containers need replicas.
deny contains msg if {
    input.component.type == "container"
    input.environment.labels.env == "prod"
    not input.component.properties.replicas
    msg := sprintf("container %q: replicas required in prod", [input.component.name])
}

# No environment should exceed $1/hr.
deny contains msg if {
    input.environment.cost.hourlyRate > 1.0
    msg := sprintf("environment %q: cost $%.2f/hr exceeds limit", [input.environment.name, input.environment.cost.hourlyRate])
}
```

## Loading policies

### File-based loading

Place `.rego` files in the `data/policies/` directory (relative to the data dir). All `.rego` files are loaded on server startup.

```
data/
├── policies/
│   ├── production.rego
│   ├── cost-limits.rego
│   └── image-registry.rego
├── my-app.app.yaml
└── dev-cluster.env.yaml
```

Start the server with the data directory:

```bash
dcm serve --data-dir data
```

The server logs which policies are loaded:

```
[compliance] loaded policy production.rego
[compliance] loaded policy cost-limits.rego
[compliance] loaded policy image-registry.rego
Compliance engine loaded with policies from data/policies
```

### Multiple files

You can split policies across multiple `.rego` files. All files must use the same package (`dcm.compliance`). Rules from all files are merged and evaluated together.

## API usage

### Check compliance without deploying

Validate an application against compliance policies without creating a deployment:

```bash
curl -X POST http://localhost:8080/api/v1/compliance/check \
  -H 'Content-Type: application/json' \
  -d '{"application": "my-web-app"}'
```

**Response — all checks pass:**

```json
{
  "application": "my-web-app",
  "violations": [],
  "passed": true
}
```

**Response — violations found:**

```json
{
  "application": "my-web-app",
  "violations": [
    {
      "rule": "",
      "message": "postgres \"database\" in prod requires at least 10Gi storage, got 1Gi"
    },
    {
      "rule": "",
      "message": "container \"backend\" in prod must specify replicas"
    }
  ],
  "passed": false
}
```

### Compliance during deployment

When compliance policies are loaded, every deployment is automatically checked after planning. If violations are found, the deployment fails with status `failed` and the violations are recorded in the deployment history.

```bash
# Create a deployment — compliance is checked automatically
curl -X POST http://localhost:8080/api/v1/deployments \
  -H 'Content-Type: application/json' \
  -d '{"application": "my-web-app"}'

# Check deployment status
curl http://localhost:8080/api/v1/deployments/{id}
```

If the compliance check fails:

```json
{
  "id": "dep-...",
  "status": "failed",
  "error": "compliance check failed: 2 violation(s)"
}
```

The deployment history includes a `compliance_failed` entry with the violation details:

```bash
curl http://localhost:8080/api/v1/deployments/{id}/history
```

```json
[
  {"action": "created", "...": "..."},
  {"action": "planning", "...": "..."},
  {"action": "planned", "...": "..."},
  {
    "action": "compliance_failed",
    "details": {
      "violations": [
        "postgres \"database\" in prod requires at least 10Gi storage, got 1Gi",
        "container \"backend\" in prod must specify replicas"
      ]
    }
  },
  {
    "action": "failed",
    "details": {"error": "compliance check failed: 2 violation(s)"}
  }
]
```

## Rego v1 syntax

DCM uses OPA v1, which requires Rego v1 syntax. Key differences from older Rego:

| v0 (old) | v1 (required) |
|----------|---------------|
| `deny[msg] { ... }` | `deny contains msg if { ... }` |
| `allow { ... }` | `allow if { ... }` |

The `contains` keyword is required for partial set rules, and the `if` keyword is required before rule bodies. Using old syntax will cause parse errors.

## Architecture

```
pkg/compliance/
├── opa.go          # Engine, Evaluate, EvaluateAll, LoadDir, LoadModule
└── opa_test.go     # Tests with sample policies

data/policies/
└── production.rego # Sample production policy
```

The `Engine` struct holds loaded Rego modules and evaluates them using OPA's Go SDK. It is created once at server startup and shared across all deployments.

### Engine methods

| Method | Description |
|--------|-------------|
| `NewEngine()` | Create engine with no policies |
| `LoadDir(dir)` | Load all `.rego` files from a directory |
| `LoadModule(name, source)` | Add a single Rego module by name and source |
| `HasPolicies()` | Returns true if any policies are loaded |
| `Evaluate(ctx, input)` | Check one step input against all policies |
| `EvaluateAll(ctx, inputs)` | Check multiple step inputs, return all violations |
