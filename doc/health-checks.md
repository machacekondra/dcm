# Environment Health Checks

DCM actively probes registered environments to track their health. A background goroutine periodically contacts each environment's health check endpoint, and the scheduler automatically excludes unhealthy environments from placement.

## How it works

```
DCM Server                          Environment
     │                                   │
     ├── GET /healthz ──────────────────▶│
     │                                   │── 200 OK
     │◀──────────────────────────────────┤
     │   status = healthy                │
     │                                   │
     │   (every 30s)                     │
     │                                   │
     ├── GET /healthz ──────────────────▶│
     │                                   │── connection refused
     │◀──────────────────────────────────┤
     │   status = unhealthy              │
     │                                   │
     │                                   │
     │       deploy request             Scheduler
     │──────────────────────────────────▶│
     │                                   ├── filter out
     │                                   │   unhealthy envs
     │                                   ├── select from
     │                                   │   healthy candidates
```

1. A background goroutine probes every environment every 30 seconds
2. **Kubernetes environments** are probed automatically using the `kubeconfig` from their config — no extra setup needed
3. **Other providers** use an explicit `healthCheck` URL if configured
4. DCM records the health status and timestamp in the database
5. DCM updates the in-memory registry so the scheduler sees changes immediately
6. During placement, the scheduler filters out `unhealthy` environments

## Health statuses

| Status | Color (UI) | Scheduler behavior | How it's set |
|--------|------------|-------------------|-------------|
| `healthy` | Green | Eligible for placement | K8s `/healthz` returns "ok", or HTTP probe returns 2xx |
| `degraded` | Orange | Eligible for placement | K8s `/healthz` returns non-"ok", or HTTP probe returns 3xx/4xx |
| `unhealthy` | Red | Excluded from placement | Probe fails, connection error, or HTTP 5xx |
| `unknown` | Grey | Eligible for placement | No health check configured (default) |

Only `unhealthy` environments are excluded from placement. `degraded` environments remain eligible — the assumption is that a degraded environment can still run workloads, just not optimally.

## Kubernetes environments (automatic)

Kubernetes environments are probed automatically — DCM uses the `kubeconfig` from the environment's `config` to call the cluster's `/healthz` endpoint. No additional health check configuration is needed.

```bash
curl -X POST http://localhost:8080/api/v1/environments \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "prod-cluster",
    "provider": "kubernetes",
    "config": {
      "kubeconfig": "/path/to/kubeconfig"
    }
  }'
```

DCM will automatically:
- Build a Kubernetes client from the kubeconfig
- Call `GET /healthz` on the cluster API server every 30 seconds
- Mark the environment as `healthy` if the response is "ok"
- Mark it as `degraded` if the cluster responds with something else
- Mark it as `unhealthy` if the connection fails

## Other providers (explicit health check)

For non-Kubernetes providers, add a `healthCheck` block when creating or updating an environment:

### Via API

```bash
curl -X POST http://localhost:8080/api/v1/environments \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "prod-cluster",
    "provider": "kubernetes",
    "config": {
      "kubeconfig": "/path/to/kubeconfig"
    },
    "healthCheck": {
      "url": "https://k8s-cluster:6443/healthz",
      "intervalSeconds": 30,
      "timeoutSeconds": 10,
      "insecureSkipVerify": true,
      "headers": {
        "Authorization": "Bearer <token>"
      }
    }
  }'
```

### Via UI

In the Create/Edit Environment modal, fill in the **Health Check** section:
- **Probe URL** — the HTTP endpoint DCM will GET
- **Interval** — how often to probe (default: 30 seconds)
- **Timeout** — HTTP timeout per probe (default: 10 seconds)
- **Skip TLS verification** — for self-signed certificates

### Health check fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | — | HTTP endpoint to probe |
| `intervalSeconds` | int | no | 30 | Probe interval in seconds |
| `timeoutSeconds` | int | no | 10 | HTTP timeout per probe in seconds |
| `insecureSkipVerify` | bool | no | false | Skip TLS certificate verification |
| `headers` | object | no | — | Additional HTTP headers (e.g., `Authorization`) |

## Probe behavior

The health checker determines status based on the HTTP response:

| Response | Status |
|----------|--------|
| HTTP 2xx | `healthy` |
| HTTP 3xx/4xx | `degraded` |
| HTTP 5xx | `unhealthy` |
| Connection error / timeout | `unhealthy` |

## Common health check URLs (non-Kubernetes)

| Provider | URL | Notes |
|----------|-----|-------|
| PostgreSQL (PgBouncer) | `http://<host>:8080/health` | If using a health proxy |
| Custom HTTP service | `http://<host>/healthz` | Any HTTP endpoint |

> Kubernetes environments don't need an explicit URL — DCM uses the kubeconfig automatically.

## Manual override

The heartbeat API is still available for manual status overrides:

```
POST /api/v1/environments/{name}/heartbeat
```

```bash
# Force an environment unhealthy for maintenance
curl -X POST http://localhost:8080/api/v1/environments/prod-cluster/heartbeat \
  -H 'Content-Type: application/json' \
  -d '{"status": "unhealthy", "message": "maintenance window"}'
```

The manual status will be overwritten on the next probe cycle if a health check is configured. To prevent this, either remove the health check config or set the environment status to `inactive`.

## Query health status

```
GET /api/v1/environments/{name}/health
```

```json
{
  "name": "prod-cluster",
  "healthStatus": "healthy",
  "healthMessage": "",
  "lastHeartbeat": "2026-03-11T15:30:00Z"
}
```

## Effect on scheduling

When a deployment is created, the scheduler:

1. Finds all environments that support the required resource type
2. Filters out environments that are `unhealthy`
3. Applies placement rules and other filters
4. Selects the best environment from remaining candidates

If all candidate environments are unhealthy, the deployment fails with:

```
no healthy environment available for component "web" (type container)
```

## UI

The Environments page shows a **Health** column with color-coded labels:

- **Green** — `healthy`
- **Orange** — `degraded`
- **Red** — `unhealthy`
- **Grey** — `unknown`

Hovering over the label shows a tooltip with the health message and last probe timestamp.

The Create/Edit modals include a **Health Check** section for configuring the probe URL and options.

## Database schema

Health check data is stored in four columns on the `environments` table:

| Column | Type | Default | Description |
|--------|------|---------|-------------|
| `health_check` | TEXT | NULL | JSON health check configuration |
| `health_status` | TEXT | `unknown` | Current health status |
| `health_message` | TEXT | `""` | Human-readable status message |
| `last_heartbeat` | TEXT | NULL | RFC3339 timestamp of last probe |

These columns are added via automatic migration on server startup.
