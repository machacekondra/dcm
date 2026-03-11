# DCM API Reference

Base URL: `http://localhost:8080/api/v1`

Start the server:

```bash
dcm serve --addr :8080 --db dcm.db
```

---

## Applications

### Create Application

```
POST /api/v1/applications
```

Creates a new application. The component dependency graph is validated on creation.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique application name |
| `labels` | object | no | Key-value labels for policy matching |
| `components` | array | yes | List of components (at least one) |

Each component:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique name within the application |
| `type` | string | yes | Resource type (`container`, `postgres`, `redis`, `static-site`, etc.) |
| `dependsOn` | string[] | no | Names of components this one depends on |
| `labels` | object | no | Key-value labels for policy matching |
| `properties` | object | no | Provider-specific configuration |

**Example**

```bash
curl -X POST http://localhost:8080/api/v1/applications \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "my-web-app",
    "labels": {"env": "production", "team": "platform"},
    "components": [
      {
        "name": "database",
        "type": "postgres",
        "labels": {"tier": "critical"},
        "properties": {"version": "16", "storage": "50Gi"}
      },
      {
        "name": "cache",
        "type": "redis",
        "properties": {"version": "7"}
      },
      {
        "name": "backend",
        "type": "container",
        "dependsOn": ["database", "cache"],
        "properties": {"image": "myapp/backend:latest", "replicas": 3}
      },
      {
        "name": "frontend",
        "type": "static-site",
        "dependsOn": ["backend"],
        "properties": {"source": "./frontend"}
      }
    ]
  }'
```

**Response** `201 Created`

```json
{
  "name": "my-web-app",
  "labels": {"env": "production", "team": "platform"},
  "components": [...],
  "createdAt": "2026-03-06T10:00:00Z",
  "updatedAt": "2026-03-06T10:00:00Z"
}
```

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | Missing name, no components, invalid DAG (unknown dependency, cycle, duplicate names) |
| `500` | Application name already exists |

---

### List Applications

```
GET /api/v1/applications
```

Returns all applications, sorted by name.

**Response** `200 OK`

```json
[
  {
    "name": "my-web-app",
    "labels": {"env": "production"},
    "components": [...],
    "createdAt": "2026-03-06T10:00:00Z",
    "updatedAt": "2026-03-06T10:00:00Z"
  }
]
```

Returns an empty array `[]` if no applications exist.

---

### Get Application

```
GET /api/v1/applications/{name}
```

**Response** `200 OK`

Returns the full application record.

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Application not found |

---

### Update Application

```
PUT /api/v1/applications/{name}
```

Replaces the application's labels and components. The component graph is re-validated.

**Request Body**

Same fields as create (`labels`, `components`). The `name` field in the body is ignored — the URL path determines the target.

**Response** `200 OK`

Returns the updated application record.

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | No components, invalid DAG |
| `404` | Application not found |

---

### Delete Application

```
DELETE /api/v1/applications/{name}
```

Deletes an application. Fails if the application has any active deployments (status not `destroyed` or `failed`).

**Response** `204 No Content`

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Application not found |
| `500` | Application has active deployments |

---

### Validate Application

```
POST /api/v1/applications/{name}/validate
```

Validates the stored application's component graph (dependency resolution, cycle detection).

**Response** `200 OK`

```json
{
  "valid": true,
  "errors": []
}
```

Or on failure:

```json
{
  "valid": false,
  "errors": ["DAG: component \"web\" depends on unknown component \"missing\""]
}
```

---

## Policies

### Create Policy

```
POST /api/v1/policies
```

Creates a new policy. CEL expressions are validated on creation.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique policy name |
| `rules` | array | yes | List of policy rules |

Each rule:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | no | Human-readable rule name |
| `priority` | integer | no | Higher values evaluate first (default: 0) |
| `match` | object | yes | Matching criteria |
| `match.type` | string | no | Match component type |
| `match.labels` | object | no | All specified labels must match |
| `match.expression` | string | no | CEL expression (see [CEL Expressions](#cel-expressions)) |
| `providers` | object | yes | Provider selection directives |
| `providers.required` | string | no | Must use this provider (error if unavailable) |
| `providers.preferred` | string[] | no | Providers in priority order |
| `providers.forbidden` | string[] | no | Providers that must not be used |
| `providers.strategy` | string | no | Selection strategy: `first`, `cheapest`, `random` |
| `properties` | object | no | Properties to inject into matching components |

**Example**

```bash
curl -X POST http://localhost:8080/api/v1/policies \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "production-placement",
    "rules": [
      {
        "name": "critical-dbs-on-aws",
        "priority": 100,
        "match": {
          "type": "postgres",
          "labels": {"tier": "critical"}
        },
        "providers": {
          "required": "aws"
        },
        "properties": {
          "multiAz": true,
          "backupRetention": "30d"
        }
      },
      {
        "name": "no-mock-in-prod",
        "priority": 200,
        "match": {
          "expression": "app.labels.env == \"production\""
        },
        "providers": {
          "forbidden": ["mock"]
        }
      }
    ]
  }'
```

**Response** `201 Created`

```json
{
  "name": "production-placement",
  "rules": [...],
  "createdAt": "2026-03-06T10:00:00Z",
  "updatedAt": "2026-03-06T10:00:00Z"
}
```

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | Missing name, invalid CEL expression |
| `500` | Policy name already exists |

---

### List Policies

```
GET /api/v1/policies
```

**Response** `200 OK`

Returns all policies sorted by name. Empty array if none exist.

---

### Get Policy

```
GET /api/v1/policies/{name}
```

**Response** `200 OK`

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Policy not found |

---

### Update Policy

```
PUT /api/v1/policies/{name}
```

Replaces the policy's rules. CEL expressions are re-validated.

**Request Body**

Same as create (`rules`).

**Response** `200 OK`

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | Invalid CEL expression |
| `404` | Policy not found |

---

### Delete Policy

```
DELETE /api/v1/policies/{name}
```

**Response** `204 No Content`

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Policy not found |

---

### Validate Policy

```
POST /api/v1/policies/{name}/validate
```

Validates the stored policy's rules and CEL expressions.

**Response** `200 OK`

```json
{
  "valid": true,
  "errors": []
}
```

---

### Evaluate Policies

```
POST /api/v1/policies/evaluate
```

Dry-run evaluation: shows which rules match each component and which provider would be selected, without deploying anything.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `application` | string | yes | Application name to evaluate against |
| `policies` | string[] | yes | Policy names to apply |

**Example**

```bash
curl -X POST http://localhost:8080/api/v1/policies/evaluate \
  -H 'Content-Type: application/json' \
  -d '{
    "application": "my-web-app",
    "policies": ["production-placement"]
  }'
```

**Response** `200 OK`

```json
[
  {
    "component": "database",
    "type": "postgres",
    "matchedRules": ["critical-dbs-on-aws"],
    "required": "aws",
    "properties": {"multiAz": true, "backupRetention": "30d"},
    "selected": "aws"
  },
  {
    "component": "cache",
    "type": "redis",
    "matchedRules": [],
    "selected": "mock"
  },
  {
    "component": "backend",
    "type": "container",
    "matchedRules": ["no-mock-in-prod"],
    "forbidden": ["mock"],
    "selected": "kubernetes"
  }
]
```

Each result may include an `error` field instead of `selected` if no provider is available.

---

## Deployments

A deployment is an instance of an application being applied to infrastructure. Each application can have at most one active deployment at a time.

### Deployment Statuses

| Status | Description |
|--------|-------------|
| `pending` | Created, waiting to start |
| `planning` | Computing the execution plan |
| `deploying` | Applying changes to providers |
| `ready` | All resources deployed successfully |
| `failed` | An error occurred (see `error` field) |
| `destroying` | Tearing down resources |
| `destroyed` | All resources removed |

### Create Deployment

```
POST /api/v1/deployments
```

Deploys an application. By default this is asynchronous — it returns immediately with `202 Accepted` and the deployment runs in the background. Poll `GET /deployments/{id}` for status updates.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `application` | string | yes | Application name to deploy |
| `policies` | string[] | no | Policy names to apply during provider selection |
| `dryRun` | boolean | no | If true, computes the plan without applying (synchronous, returns `200`) |

**Example — async deploy**

```bash
curl -X POST http://localhost:8080/api/v1/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "application": "my-web-app",
    "policies": ["production-placement"]
  }'
```

**Response** `202 Accepted`

```json
{
  "id": "dep-1772803639424825000",
  "application": "my-web-app",
  "status": "pending",
  "policies": ["production-placement"],
  "createdAt": "2026-03-06T10:00:00Z",
  "updatedAt": "2026-03-06T10:00:00Z"
}
```

**Example — dry run**

```bash
curl -X POST http://localhost:8080/api/v1/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "application": "my-web-app",
    "dryRun": true
  }'
```

**Response** `200 OK`

Returns the deployment record with status `planned` and the full plan included.

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | Missing application name |
| `404` | Application not found |
| `409` | Application already has an active deployment |
| `422` | Dry run failed (e.g., no provider available) |

---

### List Deployments

```
GET /api/v1/deployments
```

Returns all deployments, most recent first.

**Response** `200 OK`

```json
[
  {
    "id": "dep-1772803639424825000",
    "application": "my-web-app",
    "status": "ready",
    "policies": ["production-placement"],
    "createdAt": "2026-03-06T10:00:00Z",
    "updatedAt": "2026-03-06T10:01:30Z"
  }
]
```

---

### Get Deployment

```
GET /api/v1/deployments/{id}
```

Returns the full deployment record including the execution plan, resource state, and any errors. Use this endpoint to poll for deployment status.

**Response** `200 OK`

```json
{
  "id": "dep-1772803639424825000",
  "application": "my-web-app",
  "status": "ready",
  "plan": {
    "appName": "my-web-app",
    "steps": [
      {
        "component": "database",
        "diff": {
          "action": "create",
          "resource": "database",
          "type": "postgres",
          "provider": "mock",
          "after": {"version": "16", "multiAz": true}
        },
        "matchedRules": ["critical-dbs-on-aws"]
      }
    ]
  },
  "state": {
    "version": 1,
    "app": "my-web-app",
    "resources": {
      "database": {
        "name": "database",
        "type": "postgres",
        "provider": "mock",
        "status": "ready",
        "properties": {"version": "16", "multiAz": true},
        "outputs": {
          "connectionString": "postgres://user:pass@database-mock:5432/database",
          "host": "database-mock",
          "port": 5432
        }
      }
    }
  },
  "policies": ["production-placement"],
  "createdAt": "2026-03-06T10:00:00Z",
  "updatedAt": "2026-03-06T10:01:30Z"
}
```

On failure the `error` field contains the error message:

```json
{
  "id": "dep-...",
  "status": "failed",
  "error": "selecting provider for database (type postgres): no available provider for resource type \"postgres\" after applying policies"
}
```

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Deployment not found |

---

### Destroy Deployment

```
DELETE /api/v1/deployments/{id}
```

Tears down all resources for the deployment. This is asynchronous — it returns `202 Accepted` and destruction runs in the background. The deployment transitions to `destroying` and then `destroyed`.

Can only be called when the deployment is in `ready` or `failed` status.

**Response** `202 Accepted`

Returns the deployment record with status `destroying`.

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Deployment not found |
| `409` | Deployment is not in a destroyable status |

---

### Preview Plan

```
POST /api/v1/deployments/{id}/plan
```

Computes what changes would be applied if the deployment were re-applied now. Useful for seeing drift or the effect of app/policy changes on an existing deployment.

**Response** `200 OK`

```json
{
  "appName": "my-web-app",
  "steps": [
    {
      "component": "database",
      "diff": {"action": "none", "resource": "database", "type": "postgres", "provider": "mock"}
    },
    {
      "component": "backend",
      "diff": {"action": "update", "resource": "backend", "type": "container", "provider": "mock",
               "before": {"image": "myapp:v1"}, "after": {"image": "myapp:v2"}}
    }
  ]
}
```

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Deployment or application not found |
| `422` | Plan computation failed |

---

### Deployment History

```
GET /api/v1/deployments/{id}/history
```

Returns the full audit trail for a deployment, ordered chronologically.

**Response** `200 OK`

```json
[
  {
    "id": 1,
    "deploymentId": "dep-1772803639424825000",
    "action": "created",
    "details": {"policies": ["production-placement"], "dryRun": false},
    "createdAt": "2026-03-06T10:00:00Z"
  },
  {
    "id": 2,
    "deploymentId": "dep-1772803639424825000",
    "action": "planning",
    "createdAt": "2026-03-06T10:00:00Z"
  },
  {
    "id": 3,
    "deploymentId": "dep-1772803639424825000",
    "action": "planned",
    "details": {"appName": "my-web-app", "steps": [...]},
    "createdAt": "2026-03-06T10:00:01Z"
  },
  {
    "id": 4,
    "deploymentId": "dep-1772803639424825000",
    "action": "applied",
    "details": {"version": 1, "app": "my-web-app", "resources": {...}},
    "createdAt": "2026-03-06T10:00:02Z"
  }
]
```

History action types:

| Action | Description |
|--------|-------------|
| `created` | Deployment was created. Details include policies and dryRun flag. |
| `planning` | Plan computation started. |
| `planned` | Plan computed. Details contain the full execution plan. |
| `compliance_failed` | OPA compliance check failed. Details contain the violation messages. |
| `deploying` | Apply phase started. |
| `applied` | Resources applied. Details contain the full resource state. |
| `failed` | An error occurred. Details contain the error message. |
| `destroying` | Destruction started. |
| `destroyed` | All resources removed. |

**Errors**

| Status | Condition |
|--------|-----------|
| `404` | Deployment not found |

---

## Compliance

### Check Compliance

```
POST /api/v1/compliance/check
```

Validates an application against all loaded OPA compliance policies without creating a deployment. This computes a plan internally and evaluates each step against the Rego policies.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `application` | string | yes | Application name to check |

**Example**

```bash
curl -X POST http://localhost:8080/api/v1/compliance/check \
  -H 'Content-Type: application/json' \
  -d '{"application": "my-web-app"}'
```

**Response** `200 OK`

```json
{
  "application": "my-web-app",
  "violations": [],
  "passed": true
}
```

When violations are found:

```json
{
  "application": "my-web-app",
  "violations": [
    {
      "rule": "",
      "message": "postgres \"database\" in prod requires at least 10Gi storage, got 1Gi"
    }
  ],
  "passed": false
}
```

If no compliance policies are loaded, returns an empty violations list with a message:

```json
{
  "violations": [],
  "message": "no compliance policies loaded"
}
```

**Errors**

| Status | Condition |
|--------|-----------|
| `400` | Missing application name |
| `404` | Application not found |
| `422` | Plan computation failed |
| `500` | OPA evaluation error |

> See [Compliance Engine](compliance.md) for details on writing Rego policies and the input schema.

---

## Providers

### List Providers

```
GET /api/v1/providers
```

Returns all registered providers and the resource types they support.

**Response** `200 OK`

```json
[
  {
    "name": "mock",
    "capabilities": ["container", "postgres", "redis", "static-site", "network", "storage"]
  },
  {
    "name": "kubernetes",
    "capabilities": ["container"]
  }
]
```

---

## CEL Expressions

Policy rules support [CEL (Common Expression Language)](https://github.com/google/cel-spec) for advanced matching via the `match.expression` field. Expressions must evaluate to a boolean.

### Available Variables

| Variable | Type | Description |
|----------|------|-------------|
| `component.name` | string | Component name |
| `component.type` | string | Component type |
| `component.labels` | map(string, string) | Component labels |
| `component.properties` | map(string, dyn) | Component properties |
| `app.name` | string | Application name |
| `app.labels` | map(string, string) | Application labels (merged with component labels) |

### Examples

```cel
// Match containers named "backend"
component.type == "container" && component.name == "backend"

// Match production applications
app.labels.env == "production"

// Match stateful services
component.type == "postgres" || component.type == "redis"

// Match components with a specific label
component.labels.tier == "critical"
```

CEL expressions can be combined with `match.type` and `match.labels` — all conditions must be true for the rule to match.

---

## Provider Selection Logic

When policies are applied, the provider for each component is selected using this priority:

1. **Required** — If any matching rule sets `providers.required`, that provider is used. An error is raised if the provider is not registered or does not support the resource type.

2. **Preferred** — The `providers.preferred` lists from all matching rules are merged in priority order. The first provider that is registered and supports the resource type is selected.

3. **Fallback** — If no preferred provider matches, any registered provider that supports the resource type and is not in the `providers.forbidden` list is used.

The `providers.forbidden` list always takes effect — forbidden providers are removed from the preferred list and excluded from fallback selection. A conflict error is raised if a provider is both required and forbidden.

---

## Error Format

All error responses follow this format:

```json
{
  "error": "description of what went wrong"
}
```

## CORS

The API server includes permissive CORS headers (`Access-Control-Allow-Origin: *`) for browser-based UI consumption.
