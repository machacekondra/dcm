# Applications

An application in DCM is a collection of **components** that together form a deployable unit. Components have types (e.g., `container`, `postgres`), configuration properties, and dependency relationships that DCM uses to determine deployment order.

## Application structure

An application definition has four parts:

| Field | Description |
|---|---|
| `apiVersion` | API version, always `dcm.io/v1` |
| `kind` | Resource kind, always `Application` |
| `metadata` | Name, namespace, and labels |
| `spec.components` | List of components to deploy |

### Metadata

```yaml
metadata:
  name: my-web-app
  namespace: production       # optional
  labels:                     # optional, inherited by components for policy matching
    env: production
    team: platform
```

- **name** (required) — unique identifier for the application
- **namespace** (optional) — logical grouping
- **labels** (optional) — key-value pairs used for policy matching; components inherit these labels, and component-level labels override application-level labels with the same key

## Components

Each component represents a single deployable resource.

### Component fields

| Field | Required | Description |
|---|---|---|
| `name` | yes | Unique name within the application |
| `type` | yes | Resource type — determines which provider handles it |
| `dependsOn` | no | List of component names this component depends on |
| `labels` | no | Key-value pairs for policy matching |
| `properties` | no | Provider-specific configuration |

### Resource types

DCM supports the following built-in resource types:

| Type | Description | Example properties |
|---|---|---|
| `container` | Container workload (Deployment + Service on K8s) | `image`, `replicas`, `port`, `env` |
| `postgres` | PostgreSQL database | `version`, `storage`, `multiAz` |
| `redis` | Redis cache | `version`, `maxMemory` |
| `static-site` | Static website hosting | `source`, `apiEndpoint` |
| `network` | Network/VPC resources | — |
| `storage` | Object/block storage | — |

Providers declare which types they support. The mock provider supports all types; the Kubernetes provider currently supports `container`.

### Dependencies

Use `dependsOn` to declare that a component requires other components to be deployed first. DCM builds a directed acyclic graph (DAG) from these relationships and deploys components in topological order.

```yaml
components:
  - name: database
    type: postgres

  - name: backend
    type: container
    dependsOn: [database]      # deployed after database

  - name: frontend
    type: container
    dependsOn: [backend]       # deployed after backend
```

#### Dependency rules

- All names in `dependsOn` must refer to components defined in the same application
- Circular dependencies are not allowed — DCM detects cycles and reports an error
- Components with no dependencies (or whose dependencies are all satisfied) can be deployed in parallel

#### Parallel execution levels

DCM computes execution levels from the dependency graph. Components within the same level have no dependencies on each other:

```
Level 0: database, cache       (no dependencies — deployed in parallel)
Level 1: backend               (depends on database and cache)
Level 2: frontend              (depends on backend)
```

### Properties

Properties are provider-specific key-value pairs that configure the component. They are passed directly to the provider during planning and deployment.

```yaml
properties:
  image: myapp/backend:latest
  replicas: 3
  port: 8080
  env:
    DATABASE_URL: "{{ components.database.outputs.connectionString }}"
```

Properties can also be injected or overridden by [policies](policy.md). When a policy injects a property, component-level properties take precedence — policies only fill in values not already set by the component.

### Output references

Components can reference outputs from their dependencies using template syntax:

```
{{ components.<name>.outputs.<key> }}
```

For example, a backend component can reference a database connection string:

```yaml
- name: backend
  type: container
  dependsOn: [database]
  properties:
    env:
      DATABASE_URL: "{{ components.database.outputs.connectionString }}"
```

Available outputs depend on the provider. The mock provider generates outputs like `connectionString`, `endpoint`, and `url` depending on the resource type.

## Validation

DCM validates applications on creation and update:

1. **Name required** — the application must have a name
2. **At least one component** — empty component lists are rejected
3. **Unique component names** — duplicate names within the same application are not allowed
4. **Valid dependencies** — all `dependsOn` entries must reference existing components
5. **No cycles** — the dependency graph must be acyclic

Validation errors are returned immediately and prevent the application from being saved.

## Full example

```yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: my-web-app
  labels:
    env: production
    team: platform
spec:
  components:
    - name: database
      type: postgres
      labels:
        tier: critical
        data-residency: eu
      properties:
        version: "16"
        storage: 50Gi

    - name: cache
      type: redis
      properties:
        version: "7"
        maxMemory: 1Gi

    - name: backend
      type: container
      dependsOn: [database, cache]
      properties:
        image: myapp/backend:latest
        replicas: 3
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

## CLI usage

### Plan a deployment

Preview what DCM would do without making changes:

```bash
dcm plan -f app.yaml
```

With policies:

```bash
dcm plan -f app.yaml -p policies/
```

### Deploy

Apply the plan and create resources:

```bash
dcm apply -f app.yaml
dcm apply -f app.yaml -p policies/
```

### Check status

View the current state of deployed resources:

```bash
dcm status -f app.yaml
```

### Destroy

Remove all resources created by the application:

```bash
dcm destroy -f app.yaml
```

## API usage

### Create an application

```bash
curl -X POST http://localhost:8080/api/applications \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-web-app",
    "labels": {"env": "production"},
    "components": [
      {
        "name": "database",
        "type": "postgres",
        "properties": {"version": "16", "storage": "50Gi"}
      },
      {
        "name": "backend",
        "type": "container",
        "dependsOn": ["database"],
        "properties": {"image": "myapp/backend:latest", "replicas": 3}
      }
    ]
  }'
```

### List applications

```bash
curl http://localhost:8080/api/applications
```

### Get an application

```bash
curl http://localhost:8080/api/applications/my-web-app
```

### Update an application

```bash
curl -X PUT http://localhost:8080/api/applications/my-web-app \
  -H 'Content-Type: application/json' \
  -d '{
    "labels": {"env": "staging"},
    "components": [
      {"name": "web", "type": "container", "properties": {"image": "nginx:latest"}}
    ]
  }'
```

### Delete an application

Applications with active deployments cannot be deleted. Destroy the deployment first.

```bash
curl -X DELETE http://localhost:8080/api/applications/my-web-app
```

### Validate an application

Check if a stored application's component graph is valid:

```bash
curl http://localhost:8080/api/applications/my-web-app/validate
```

Response:

```json
{
  "valid": true,
  "errors": []
}
```

## Deployment lifecycle

When an application is deployed (via CLI or API), DCM:

1. Builds the dependency DAG and computes topological order
2. Evaluates policies (if any) to determine provider selection and property injection for each component
3. Runs the planner — each component is diffed against current state (create/update/delete/none)
4. Executes the plan — components are applied in dependency order through their assigned providers
5. Stores the resulting state — resource outputs, statuses, and provider assignments

Only one deployment per application is allowed at a time. To redeploy, destroy the existing deployment first.
