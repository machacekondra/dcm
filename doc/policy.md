# Policy Engine

DCM's policy engine controls **provider selection** and **property injection** for application components. Policies let you define rules like "production databases must use AWS" or "never use the mock provider in staging" without modifying application definitions.

## Concepts

### Policy

A policy is a named collection of rules. Multiple policies can be loaded simultaneously — all rules from all policies are merged and evaluated together.

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: production-placement
spec:
  rules:
    - name: prefer-aws-for-databases
      priority: 100
      match:
        type: postgres
      providers:
        preferred: [aws]
```

### Rule

Each rule has three parts:

1. **Match** — conditions that determine which components the rule applies to
2. **Providers** — provider selection constraints (preferred, required, forbidden)
3. **Properties** — key-value pairs merged into matching component properties

### Priority

Every rule has a numeric priority (default: 0). Higher numbers are evaluated first. When multiple rules match the same component:

- **Required**: first (highest-priority) match wins
- **Strategy**: first match wins
- **Preferred**: lists are concatenated in priority order, then deduplicated
- **Forbidden**: always accumulated from all matching rules
- **Properties**: higher-priority values take precedence; lower-priority values fill gaps

## Matching

Rules match components using three mechanisms, all of which must pass for the rule to apply (AND logic).

### Type matching

Matches against the component's `type` field (e.g., `postgres`, `container`, `redis`).

```yaml
match:
  type: postgres
```

### Label matching

Matches against the merged labels of the component and its parent application. All specified labels must be present (AND logic). Component labels override application labels when both define the same key.

```yaml
match:
  labels:
    env: production
    tier: critical
```

### CEL expressions

For advanced matching, use [Common Expression Language (CEL)](https://github.com/google/cel-go) expressions. The expression must evaluate to `true` for the rule to match.

```yaml
match:
  expression: 'component.type == "container" && component.name == "backend"'
```

#### Available CEL variables

| Variable | Type | Description |
|---|---|---|
| `component.name` | string | Component name |
| `component.type` | string | Component type (e.g., "postgres") |
| `component.labels` | map[string, string] | Component labels |
| `component.properties` | map[string, dyn] | Component properties |
| `app.name` | string | Application name |
| `app.labels` | map[string, string] | Application labels |

#### CEL expression examples

```yaml
# Match containers named "backend"
expression: 'component.type == "container" && component.name == "backend"'

# Match production applications
expression: 'app.labels.env == "production"'

# Match stateful services
expression: 'component.type == "postgres" || component.type == "redis"'

# Match components with specific properties
expression: 'has(component.properties.replicas) && component.properties.replicas > 3'
```

### Combining match criteria

When multiple criteria are specified, all must match:

```yaml
match:
  type: postgres                    # AND
  labels:                           # AND
    env: production
  expression: 'component.name != "test-db"'   # all three must be true
```

## Provider selection

Each rule can specify provider constraints using the `providers` field.

### Preferred

An ordered list of providers to try. The first registered provider that supports the component's resource type is selected.

```yaml
providers:
  preferred: [aws, gcp, azure]
```

### Required

Forces a specific provider. If the provider is not registered or doesn't support the resource type, deployment fails with an error.

```yaml
providers:
  required: aws
```

### Forbidden

Providers that must never be used. If a provider appears in both preferred and forbidden lists, it is removed from preferred. If a required provider is also forbidden, evaluation fails with a conflict error.

```yaml
providers:
  forbidden: [mock]
```

### Strategy

Selects the algorithm when multiple providers are available: `first`, `cheapest`, or `random`.

```yaml
providers:
  strategy: cheapest
```

### Selection algorithm

When deploying a component, the engine selects a provider in this order:

1. **Required** — if set, use that provider (error if unavailable)
2. **Preferred** — walk the list, return the first registered provider that supports the type
3. **Fallback** — pick any registered, non-forbidden provider that supports the type
4. **Error** — if no provider is available after filtering

## Property injection

Rules can inject or override component properties. Higher-priority rules take precedence.

```yaml
rules:
  - name: production-postgres
    priority: 100
    match:
      type: postgres
      labels:
        env: production
    providers:
      preferred: [aws]
    properties:
      multiAz: true
      backupRetention: 30d

  - name: default-postgres
    priority: 50
    match:
      type: postgres
    properties:
      backupRetention: 7d
      version: "16"
```

For a production postgres component, the merged properties would be:
- `multiAz: true` (from priority 100)
- `backupRetention: 30d` (from priority 100, overrides the priority-50 value)
- `version: "16"` (from priority 50, not overridden)

## Conflict detection

The engine detects and reports conflicts:

- **Required + Forbidden**: if a provider is both required and forbidden for the same component, evaluation fails with an error
- **Forbidden removes Preferred**: if a preferred provider is also forbidden by any rule, it is silently removed from the preferred list

## Examples

### Label-based placement

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: production-placement
spec:
  rules:
    - name: eu-data-residency
      priority: 100
      match:
        labels:
          env: production
          data-residency: eu
      providers:
        preferred: [aws-eu-west-1, gcp-europe-west1]
        forbidden: [aws-us-east-1]

    - name: critical-postgres-on-aws
      priority: 90
      match:
        type: postgres
        labels:
          tier: critical
      providers:
        preferred: [aws]
      properties:
        multiAz: true
        backupRetention: 30d

    - name: cost-optimized-containers
      priority: 50
      match:
        type: container
        labels:
          cost: optimize
      providers:
        strategy: cheapest
```

### CEL-based rules

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: cel-based-rules
spec:
  rules:
    - name: backend-to-kubernetes
      priority: 80
      match:
        expression: 'component.type == "container" && component.name == "backend"'
      providers:
        preferred: [kubernetes]

    - name: production-no-mock
      priority: 200
      match:
        expression: 'app.labels.env == "production"'
      providers:
        forbidden: [mock]

    - name: stateful-services-prefer-aws
      priority: 70
      match:
        expression: 'component.type == "postgres" || component.type == "redis"'
      providers:
        preferred: [aws]
```

## CLI usage

### Validate policies

Check policy files for syntax errors and invalid CEL expressions:

```bash
dcm policy validate -p examples/policies/
dcm policy validate -p placement.yaml -p cel-rules.yaml
```

### List policies

Show all loaded policies and their rules:

```bash
dcm policy list -p examples/policies/
```

### Evaluate policies

Run policies against an application to see which rules match and which providers would be selected:

```bash
dcm policy evaluate -f examples/web-app/app.yaml -p examples/policies/
```

Example output:

```
Policy evaluation for application: web-app
═══════════════════════════════════════════════════════

  Component: database (type: postgres)
    Matched rules:
      - stateful-services-prefer-aws
    Preferred providers: [aws]
    Selected provider: mock

  Component: cache (type: redis)
    Matched rules:
      - stateful-services-prefer-aws
    Preferred providers: [aws]
    Selected provider: mock

  Component: backend (type: container)
    Matched rules:
      - backend-to-kubernetes
    Preferred providers: [kubernetes]
    Selected provider: mock

  Component: frontend (type: container)
    Matched rules: (none)
    Selected provider: mock

═══════════════════════════════════════════════════════
```

### Use policies during deployment

Pass policies to `plan` and `apply` commands:

```bash
dcm plan -f app.yaml -p policies/
dcm apply -f app.yaml -p policies/
```

## API usage

### Create a policy

```bash
curl -X POST http://localhost:8080/api/policies \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "production-placement",
    "rules": [
      {
        "name": "no-mock-in-prod",
        "priority": 200,
        "match": { "expression": "app.labels.env == \"production\"" },
        "providers": { "forbidden": ["mock"] }
      }
    ]
  }'
```

### Evaluate policies against a component

```bash
curl -X POST http://localhost:8080/api/evaluate \
  -H 'Content-Type: application/json' \
  -d '{
    "component": { "name": "db", "type": "postgres", "labels": {"tier": "critical"} },
    "application": "my-app"
  }'
```

### Deploy with policies

```bash
curl -X POST http://localhost:8080/api/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "application": "my-app",
    "policies": ["production-placement", "cel-based-rules"]
  }'
```

## Multi-policy evaluation

When multiple policies are loaded, all rules from all policies are collected, sorted by priority (descending), and evaluated as a single flat list. Rules with the same priority preserve their declaration order. This means a high-priority rule in one policy always takes precedence over a lower-priority rule in another policy, regardless of load order.
