# Requirements & Capabilities

## Problem

Components often need specific infrastructure features from their target environment.
A container that needs an external IP requires a cluster with MetalLB or a cloud load
balancer. A database that needs persistent storage requires an environment with a CSI
driver. Today, the scheduler picks environments based on provider type and policy
strategy alone — it has no way to know whether an environment actually provides what a
component needs.

Additionally, some components must land on the **same** environment. A container and the
IP address assigned to it must be on the same Kubernetes cluster, but a DNS record
managed by an external API (PowerDNS) does not need co-location.

## Concepts

### Capabilities (on environments)

An environment declares a list of string tags representing what it can provide:

```yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: prod-k8s
spec:
  provider: kubernetes
  capabilities:
    - loadbalancer
    - persistent-storage
    - gpu
  config:
    kubeconfig: /path/to/kubeconfig
```

Capabilities are opaque strings. DCM does not interpret them — they are matched
literally against component requirements. Teams choose their own vocabulary.

Common examples:

| Capability | Meaning |
|---|---|
| `loadbalancer` | MetalLB, cloud LB, or similar |
| `persistent-storage` | CSI driver / StorageClass available |
| `gpu` | GPU nodes available |
| `ingress` | Ingress controller installed |
| `multicluster` | Federation / multi-cluster networking |
| `ipv6` | IPv6 networking support |

### Requirements (on components)

A component declares which capabilities it needs. The scheduler only considers
environments that have **all** listed capabilities:

```yaml
components:
  - name: app
    type: container
    requires: [loadbalancer]
    properties:
      image: myapp:latest
```

If no environment satisfies the requirements, planning fails with a clear error.

### Co-location (colocateWith)

`colocateWith` names another component that must be scheduled to the **same**
environment. This is separate from `dependsOn`:

- `dependsOn` = **ordering** — create A before B
- `colocateWith` = **placement** — A and B must be on the same environment

```yaml
components:
  - name: app
    type: container
    requires: [loadbalancer]
    properties:
      image: myapp:latest

  - name: app-ip
    type: ip
    dependsOn: [app]
    requires: [loadbalancer]
    colocateWith: app
    properties:
      pool: production

  - name: app-dns
    type: dns
    dependsOn: [app-ip]
    properties:
      zone: example.com
      record: myapp.example.com
      type: A
      value: "${app-ip.outputs.address}"
```

Here `app-dns` has no `colocateWith` — it talks to an external DNS API and does not
need to be on the same cluster.

## Placement Groups

Components linked via `colocateWith` form a **placement group**. All members of a group
are scheduled to the same environment.

### Group construction

1. Parse all `colocateWith` links into an undirected graph.
2. Find connected components — each connected component is a placement group.
3. Components with no `colocateWith` form singleton groups.

Example:

```
app                      (requires: [loadbalancer])
app-ip   colocateWith: app   (requires: [loadbalancer])
database colocateWith: app   (requires: [persistent-storage])
app-dns                  (requires: [])
```

Groups:
- **Group 1**: `[app, app-ip, database]` — merged requirements: `[loadbalancer, persistent-storage]`
- **Group 2**: `[app-dns]` — no requirements

### Transitive linking

`colocateWith` is transitive. If A colocates with B and C colocates with B, then
A, B, and C are all in the same group. Circular references are fine — they just
confirm the same group membership.

## Scheduling Flow

```
1. Build placement groups from colocateWith links.

2. For each group, compute merged requirements (union of all members).

3. For each group, filter candidate environments:
   - Must have a provider that supports the component types in the group
   - Must have ALL merged capabilities

4. Apply policy/strategy (first, round-robin, cheapest, etc.) to pick
   from filtered candidates.

5. All components in the group are assigned the chosen environment.
```

### Scheduler changes

The current scheduler processes components individually:

```
for each component:
    candidates = environments supporting component.type
    chosen = applyStrategy(candidates)
```

With requirements, this becomes:

```
groups = buildPlacementGroups(components)

for each group:
    mergedRequirements = union of all member requirements
    mergedTypes = union of all member types

    candidates = environments supporting ALL mergedTypes
    candidates = filter(candidates, hasAll(mergedRequirements))

    if len(candidates) == 0:
        error: "no environment satisfies [requirements] for group [members]"

    chosen = applyStrategy(candidates)

    for each component in group:
        component.environment = chosen
```

## Type Changes

### Component

```go
type Component struct {
    Name         string            `json:"name" yaml:"name"`
    Type         string            `json:"type" yaml:"type"`
    DependsOn    []string          `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
    Requires     []string          `json:"requires,omitempty" yaml:"requires,omitempty"`
    ColocateWith string            `json:"colocateWith,omitempty" yaml:"colocateWith,omitempty"`
    Labels       map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
    Properties   map[string]any    `json:"properties,omitempty" yaml:"properties,omitempty"`
}
```

### EnvironmentSpec

```go
type EnvironmentSpec struct {
    Provider     string         `json:"provider" yaml:"provider"`
    Capabilities []string       `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
    Config       map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
    Resources    *ResourcePool  `json:"resources,omitempty" yaml:"resources,omitempty"`
    Cost         *CostInfo      `json:"cost,omitempty" yaml:"cost,omitempty"`
}
```

### Store (database)

`environments` table gains a `capabilities` column (JSON array of strings):

```sql
ALTER TABLE environments ADD COLUMN capabilities TEXT DEFAULT '[]';
```

Stored as `["loadbalancer", "persistent-storage"]`.

## Validation

| Check | When | Error |
|---|---|---|
| `colocateWith` target does not exist | Plan time | `component "app-ip": colocateWith target "app" not found` |
| No environment satisfies requirements | Plan time | `no environment provides [gpu] for placement group [ml-worker]` |
| `colocateWith` creates a group with incompatible types | Plan time | `no environment supports both [container, vm] for group [...]` |
| Empty `requires` list | Always valid | Component runs on any environment |
| Capability not used by any component | Always valid | No warning (capabilities are declarative) |

## Full Example — Kubernetes with MetalLB

### Environments

```yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: onprem-k8s
  labels:
    region: dc-east
spec:
  provider: kubernetes
  capabilities:
    - loadbalancer
    - persistent-storage
  config:
    kubeconfig: /etc/rancher/k3s/k3s.yaml

---
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: dev-k8s
spec:
  provider: kubernetes
  capabilities:
    - persistent-storage
  config:
    kubeconfig: ~/.kube/dev-config
```

### Application

```yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: spring-petclinic
spec:
  components:
    - name: database
      type: postgres
      requires: [persistent-storage]
      colocateWith: app
      labels:
        tier: data
      properties:
        version: "16"
        storage: 20Gi

    - name: cache
      type: redis
      colocateWith: app
      labels:
        tier: data
      properties:
        version: "7"

    - name: app
      type: container
      dependsOn: [database, cache]
      requires: [loadbalancer]
      labels:
        tier: backend
      properties:
        image: springcommunity/spring-petclinic:latest
        replicas: 2
        env:
          SPRING_DATASOURCE_URL: "${database.outputs.connectionString}"
          SPRING_DATA_REDIS_HOST: "${cache.outputs.host}"

    - name: app-ip
      type: ip
      dependsOn: [app]
      requires: [loadbalancer]
      colocateWith: app
      labels:
        tier: network
      properties:
        pool: production

    - name: app-dns
      type: dns
      dependsOn: [app-ip]
      labels:
        tier: network
      properties:
        zone: example.com
        record: petclinic.example.com
        type: A
        value: "${app-ip.outputs.address}"
```

### Plan output

```
Plan for application: spring-petclinic
-----------------------------------------
  Placement group: [database, cache, app, app-ip]
    Requirements: [loadbalancer, persistent-storage]
    Environment:  onprem-k8s (only match)

  + database   (postgres via kubernetes @ onprem-k8s)
  + cache      (redis via kubernetes @ onprem-k8s)
  + app        (container via kubernetes @ onprem-k8s)
  + app-ip     (ip via static-ipam @ onprem-k8s)
  + app-dns    (dns via powerdns)
-----------------------------------------
  5 to create, 0 to update, 0 to delete, 0 unchanged
```

`dev-k8s` is excluded because it lacks `loadbalancer`.
`app-dns` is independent — scheduled to any environment with a DNS provider.

## Implementation Order

1. **Types** — Add `Requires` and `ColocateWith` to `Component`, `Capabilities` to
   `EnvironmentSpec`.

2. **Store** — Add `capabilities` column to `environments` table. Update
   `EnvironmentRecord` and CRUD operations.

3. **Placement groups** — Build group construction logic
   (`buildPlacementGroups(components) -> []PlacementGroup`).

4. **Scheduler** — Update candidate filtering to check capabilities against merged
   requirements. Schedule groups instead of individual components.

5. **Planner** — Validate `colocateWith` targets exist. Validate requirements are
   satisfiable before generating the plan.

6. **API** — Expose capabilities in environment CRUD endpoints. Add capabilities
   to the UI environment form.

7. **Seed data** — Update `dev-cluster` seed environment with sample capabilities.
   Update `spring-petclinic` seed app with requirements.

8. **CLI output** — Show placement groups and requirements in plan output.
