# Environments — Design

## Problem

Today, DCM registers one instance per provider. If you have three Kubernetes clusters (prod-eu, prod-us, dev), you can only register one. There's no way to:

- Run multiple instances of the same provider type with different configs
- Schedule workloads to a specific cluster based on region, cost, or capacity
- Spread components across clusters for resilience
- Pick the cheapest or least-loaded cluster automatically

## Core concept

An **Environment** is a named, configured instance of a provider type. It carries metadata (labels, capacity, cost) that the scheduler uses to make placement decisions.

```
Current model:
  Provider "kubernetes" ──── one kubeconfig, one cluster

New model:
  Provider Type "kubernetes" ──── the implementation (code)
    ├── Environment "k8s-prod-eu"  ──── cluster in eu-west-1
    ├── Environment "k8s-prod-us"  ──── cluster in us-east-1
    └── Environment "k8s-dev"      ──── dev cluster (spot)
```

The Provider interface stays the same — it's the implementation contract. An Environment wraps a Provider instance with identity, config, and scheduling metadata.

## Environment definition

```yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: k8s-prod-eu
  labels:
    region: eu-west-1
    zone: eu-west-1a
    tier: production
    team: platform
spec:
  provider: kubernetes
  config:
    kubeconfig: /path/to/kubeconfig
    context: prod-eu-cluster
    namespace: production
  resources:
    cpu: 8000        # millicores available
    memory: 32768    # MB available
    pods: 200        # max pods
  cost:
    tier: standard   # standard | premium | spot
    hourlyRate: 0.05 # cost per resource-unit per hour
```

### Fields

| Field | Required | Description |
|---|---|---|
| `metadata.name` | yes | Unique environment name, used in policies and scheduling |
| `metadata.labels` | no | Key-value pairs for matching in policies and CEL expressions |
| `spec.provider` | yes | Provider type: `kubernetes`, `aws`, `mock`, etc. |
| `spec.config` | yes | Provider-specific configuration (kubeconfig, credentials, region, etc.) |
| `spec.resources` | no | Available capacity — used by `least-loaded` and `bin-pack` strategies |
| `spec.cost` | no | Cost metadata — used by `cheapest` strategy |

### Provider-specific config

**Kubernetes:**
```yaml
config:
  kubeconfig: /path/to/kubeconfig   # path to kubeconfig file
  context: my-context               # kubeconfig context to use
  namespace: default                # target namespace
```

**AWS (future):**
```yaml
config:
  region: eu-west-1
  profile: production
  accountId: "123456789"
```

**Mock:**
```yaml
config: {}   # no config needed
```

## Scheduling

The **Scheduler** replaces the current simple provider selection. It picks the best environment for each component.

### Selection flow

```
Component
  │
  ▼
┌──────────────────────┐
│ 1. Filter by type    │  Does the environment's provider support
│    (capability)      │  the component's resource type?
├──────────────────────┤
│ 2. Filter by policy  │  Apply required/preferred/forbidden
│    (provider type)   │  from policy provider rules
├──────────────────────┤
│ 3. Filter by policy  │  Apply required/preferred/forbidden
│    (environment)     │  from policy environment rules
├──────────────────────┤
│ 4. Filter by policy  │  CEL expressions can reference
│    (CEL expressions) │  environment.labels, environment.resources
├──────────────────────┤
│ 5. Optimize          │  Apply scheduling strategy to pick
│    (strategy)        │  the best from remaining candidates
└──────────────────────┘
  │
  ▼
Selected Environment
```

### Strategies

| Strategy | Description |
|---|---|
| `first` | First available environment (default, current behavior) |
| `cheapest` | Lowest `cost.hourlyRate` among candidates |
| `least-loaded` | Most available capacity (highest remaining resources) |
| `round-robin` | Distribute components evenly across environments |
| `bin-pack` | Pack into fewest environments (fill one before using next) |

## Policy integration

Policies gain a new `environments` field alongside the existing `providers` field. Both work together as filters:

- `providers` — filters by **provider type** (e.g., "any kubernetes-backed environment")
- `environments` — filters by **environment name** (e.g., "specifically k8s-prod-eu")

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: production-scheduling
spec:
  rules:
    # Rule 1: EU data must stay in EU environments
    - name: eu-data-residency
      priority: 200
      match:
        labels:
          data-residency: eu
      environments:
        preferred: [k8s-prod-eu, k8s-prod-eu-2]
        forbidden: [k8s-prod-us, k8s-staging-us]

    # Rule 2: Production workloads must not use mock
    - name: no-mock-in-prod
      priority: 150
      match:
        labels:
          env: production
      providers:
        forbidden: [mock]

    # Rule 3: Databases should go to environments with enough resources
    - name: databases-need-resources
      priority: 100
      match:
        type: postgres
      environments:
        strategy: least-loaded
      properties:
        multiAz: true

    # Rule 4: Cost-optimize dev workloads
    - name: dev-cheapest
      priority: 80
      match:
        labels:
          env: development
      environments:
        strategy: cheapest

    # Rule 5: CEL-based environment filtering
    - name: high-memory-workloads
      priority: 90
      match:
        expression: >
          component.type == "postgres" &&
          has(component.properties.storage) &&
          component.properties.storage == "100Gi"
      environments:
        matchExpression: 'environment.resources.memory > 16384'
        strategy: bin-pack
```

### Updated ProviderPolicy type

```yaml
# Existing — filters by provider TYPE
providers:
  preferred: [kubernetes, aws]
  required: kubernetes
  forbidden: [mock]

# New — filters by environment NAME and applies scheduling
environments:
  preferred: [k8s-prod-eu, k8s-prod-us]
  required: k8s-prod-eu
  forbidden: [k8s-dev]
  matchExpression: 'environment.labels.region == "eu-west-1"'  # CEL filter
  strategy: cheapest
```

### CEL extensions

New variables available in CEL expressions:

| Variable | Type | Description |
|---|---|---|
| `environment.name` | string | Environment name |
| `environment.labels` | map[string, string] | Environment labels |
| `environment.provider` | string | Provider type name |
| `environment.resources.cpu` | int | Available CPU (millicores) |
| `environment.resources.memory` | int | Available memory (MB) |
| `environment.resources.pods` | int | Available pod slots |
| `environment.cost.tier` | string | Cost tier |
| `environment.cost.hourlyRate` | double | Hourly rate |

These are available in the `environments.matchExpression` field (evaluated per-environment), not in the component-level `match.expression` field (which filters components).

## Data model changes

### New types

```go
type Environment struct {
    APIVersion string            `yaml:"apiVersion" json:"apiVersion"`
    Kind       string            `yaml:"kind" json:"kind"`
    Metadata   Metadata          `yaml:"metadata" json:"metadata"`
    Spec       EnvironmentSpec   `yaml:"spec" json:"spec"`
}

type EnvironmentSpec struct {
    Provider  string            `yaml:"provider" json:"provider"`
    Config    map[string]any    `yaml:"config" json:"config"`
    Resources *ResourcePool     `yaml:"resources,omitempty" json:"resources,omitempty"`
    Cost      *CostInfo         `yaml:"cost,omitempty" json:"cost,omitempty"`
}

type ResourcePool struct {
    CPU    int `yaml:"cpu" json:"cpu"`       // millicores
    Memory int `yaml:"memory" json:"memory"` // MB
    Pods   int `yaml:"pods" json:"pods"`
}

type CostInfo struct {
    Tier       string  `yaml:"tier" json:"tier"`
    HourlyRate float64 `yaml:"hourlyRate" json:"hourlyRate"`
}
```

### Updated PolicyRule

```go
type PolicyRule struct {
    Name         string            `yaml:"name,omitempty" json:"name,omitempty"`
    Priority     int               `yaml:"priority,omitempty" json:"priority,omitempty"`
    Match        PolicyMatch       `yaml:"match" json:"match"`
    Providers    ProviderPolicy    `yaml:"providers,omitempty" json:"providers,omitempty"`
    Environments EnvironmentPolicy `yaml:"environments,omitempty" json:"environments,omitempty"`
    Properties   map[string]any    `yaml:"properties,omitempty" json:"properties,omitempty"`
}

type EnvironmentPolicy struct {
    Preferred       []string `yaml:"preferred,omitempty" json:"preferred,omitempty"`
    Required        string   `yaml:"required,omitempty" json:"required,omitempty"`
    Forbidden       []string `yaml:"forbidden,omitempty" json:"forbidden,omitempty"`
    MatchExpression string   `yaml:"matchExpression,omitempty" json:"matchExpression,omitempty"`
    Strategy        string   `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}
```

### Registry changes

The `Registry` evolves to manage environments instead of bare providers:

```go
type EnvironmentInstance struct {
    Environment types.Environment    // metadata, config, resources, cost
    Provider    types.Provider       // the live provider instance
}

type Registry struct {
    environments map[string]*EnvironmentInstance
}

func (r *Registry) Register(env types.Environment, provider types.Provider)
func (r *Registry) Get(name string) (*EnvironmentInstance, bool)
func (r *Registry) ListByProvider(providerType string) []*EnvironmentInstance
func (r *Registry) ListByCapability(rt types.ResourceType) []*EnvironmentInstance
func (r *Registry) ListAll() []*EnvironmentInstance
```

### Store changes

New `environments` table:

```sql
CREATE TABLE environments (
    name       TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    labels     TEXT NOT NULL DEFAULT '{}',
    config     TEXT NOT NULL DEFAULT '{}',
    resources  TEXT,
    cost       TEXT,
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

Deployments record which environment each component was scheduled to:

```sql
-- Add to deployments or track per-component
ALTER TABLE deployments ADD COLUMN environment_assignments TEXT DEFAULT '{}';
-- JSON: {"database": "k8s-prod-eu", "backend": "k8s-prod-us"}
```

## Scheduler implementation

```go
type Scheduler struct {
    registry  *Registry
    evaluator *policy.Evaluator
}

type ScheduleResult struct {
    Environment  string   // selected environment name
    Provider     string   // provider type
    MatchedRules []string // policy rules that influenced the decision
    Strategy     string   // strategy used for final selection
}

func (s *Scheduler) Schedule(
    component *types.Component,
    app *types.Application,
) (*ScheduleResult, error)
```

The scheduler:
1. Gets all environments from the registry
2. Filters by capability (provider must support the component's resource type)
3. Evaluates policies to get provider-level and environment-level constraints
4. Applies `providers.forbidden` — remove environments backed by forbidden provider types
5. Applies `providers.required` — keep only environments backed by the required provider type
6. Applies `environments.forbidden` — remove forbidden environments by name
7. Applies `environments.required` — if set, use exactly that environment
8. Evaluates `environments.matchExpression` CEL against each remaining environment
9. Orders by `environments.preferred` (preferred environments first)
10. Falls back to `providers.preferred` (preferred provider types first)
11. Applies strategy (`cheapest`, `least-loaded`, `bin-pack`, etc.) among remaining candidates
12. Returns the selected environment

## Backward compatibility

- If no environments are defined, DCM auto-creates a default environment per registered provider (e.g., environment "kubernetes" backed by provider "kubernetes"). Existing policies that target provider names continue to work.
- The `providers` field on policy rules continues to work as before — it filters by provider type.
- The `environments` field is additive — old policies without it work unchanged.
- CLI commands (`plan`, `apply`) gain an optional `--environment` / `-e` flag to load environment definitions.

## Example: full multi-cluster setup

### Environments

```yaml
# environments/clusters.yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: k8s-prod-eu
  labels:
    region: eu-west-1
    tier: production
spec:
  provider: kubernetes
  config:
    kubeconfig: ~/.kube/config
    context: prod-eu
    namespace: apps
  resources:
    cpu: 16000
    memory: 65536
    pods: 500
  cost:
    tier: standard
    hourlyRate: 0.05
---
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: k8s-prod-us
  labels:
    region: us-east-1
    tier: production
spec:
  provider: kubernetes
  config:
    kubeconfig: ~/.kube/config
    context: prod-us
    namespace: apps
  resources:
    cpu: 32000
    memory: 131072
    pods: 1000
  cost:
    tier: standard
    hourlyRate: 0.04
---
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: k8s-dev
  labels:
    region: eu-west-1
    tier: development
spec:
  provider: kubernetes
  config:
    kubeconfig: ~/.kube/config
    context: dev
    namespace: dev
  resources:
    cpu: 4000
    memory: 16384
    pods: 100
  cost:
    tier: spot
    hourlyRate: 0.01
---
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: mock-local
  labels:
    tier: development
spec:
  provider: mock
  config: {}
```

### Policies

```yaml
# policies/scheduling.yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: environment-scheduling
spec:
  rules:
    - name: prod-no-dev-clusters
      priority: 200
      match:
        labels:
          env: production
      environments:
        forbidden: [k8s-dev, mock-local]

    - name: eu-data-stays-in-eu
      priority: 150
      match:
        labels:
          data-residency: eu
      environments:
        matchExpression: 'environment.labels.region == "eu-west-1"'

    - name: spread-across-clusters
      priority: 100
      match:
        type: container
      environments:
        preferred: [k8s-prod-eu, k8s-prod-us]
        strategy: least-loaded

    - name: databases-bin-pack
      priority: 100
      match:
        type: postgres
      environments:
        strategy: bin-pack
```

### Application (unchanged)

```yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: my-web-app
  labels:
    env: production
    data-residency: eu
spec:
  components:
    - name: database
      type: postgres
      properties:
        version: "16"

    - name: backend
      type: container
      dependsOn: [database]
      properties:
        image: myapp/backend:latest
        replicas: 3
```

### Deployment result

```
Component: database (type: postgres)
  Matched rules: prod-no-dev-clusters, eu-data-stays-in-eu, databases-bin-pack
  Strategy: bin-pack
  Candidates: [k8s-prod-eu]  (us filtered by eu-data, dev/mock filtered by prod rule)
  → Scheduled to: k8s-prod-eu

Component: backend (type: container)
  Matched rules: prod-no-dev-clusters, eu-data-stays-in-eu, spread-across-clusters
  Strategy: least-loaded
  Candidates: [k8s-prod-eu]  (us filtered by eu-data, dev/mock filtered by prod rule)
  → Scheduled to: k8s-prod-eu
```

## API endpoints

```
POST   /api/environments          Create environment
GET    /api/environments          List environments
GET    /api/environments/{name}   Get environment
PUT    /api/environments/{name}   Update environment
DELETE /api/environments/{name}   Delete environment
POST   /api/environments/{name}/test   Test connectivity
```

## UI additions

- **Environments page** — list all environments with provider type, labels, status, resource usage
- **Environment detail** — config, capacity gauges, deployments running on it
- **Deploy modal** — optionally target specific environment(s)
- **Deployment detail** — show which environment each component was scheduled to

## Implementation order

1. **Types** — `Environment`, `EnvironmentSpec`, `EnvironmentPolicy` structs
2. **Loader** — `LoadEnvironment()`, `LoadEnvironments()` for YAML files
3. **Registry** — refactor to manage `EnvironmentInstance` instead of bare providers
4. **Scheduler** — new package implementing the selection flow
5. **Store** — `environments` table, CRUD operations
6. **Policy** — extend evaluator for `environments` field and CEL variables
7. **Planner** — use Scheduler instead of direct provider selection
8. **API** — environment CRUD endpoints
9. **CLI** — `--environment` flag, `dcm environment` subcommands
10. **UI** — environments page, updated deploy modal
