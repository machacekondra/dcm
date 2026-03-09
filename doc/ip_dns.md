# IP/DNS Management Design

## Overview

When deploying applications, DCM needs to reserve IP addresses and manage DNS records automatically. This is modeled using two new resource types (`ip` and `dns`) that integrate with the existing component, provider, and policy system.

The key enabler is **output references** — a mechanism for downstream components to reference outputs from upstream components at apply time.

## Architecture

```
+--------------+     dependsOn      +---------------+     dependsOn     +---------------+
|  webserver   |<-------------------|  webserver-ip  |<-----------------|  webserver-dns |
|  type: vm    |                    |  type: ip      |                  |  type: dns     |
|              |                    |                |                  |                |
| outputs:     |                    | outputs:       |                  | outputs:       |
|   vmName: ...|                    |   address:     |                  |   fqdn: ...    |
|              |                    |     10.0.1.42  |                  |                |
+--------------+                    +---------------+                  +---------------+
```

Components are processed in dependency order (DAG topological sort). Each component's outputs are stored in state and available to subsequent components via `${}` references.

## Output References

### Syntax

Properties can reference outputs from other components using `${component.outputs.field}`:

```yaml
components:
  - name: webserver
    type: vm
    properties:
      image: quay.io/containerdisks/fedora:latest
      cpu: 4
      memory: 4Gi

  - name: webserver-ip
    type: ip
    dependsOn: [webserver]
    properties:
      pool: production
      attachTo: ${webserver.outputs.vmName}

  - name: webserver-dns
    type: dns
    dependsOn: [webserver-ip]
    properties:
      zone: example.com
      record: webserver.example.com
      type: A
      value: ${webserver-ip.outputs.address}
      ttl: 300
```

### Resolution

References are resolved at **apply time**, not plan time, because outputs don't exist until resources are created.

```
Planner (CreatePlan)              Executor (Execute)
------------------------          --------------------------
                                  for each step (topo order):
component.Properties                1. resolveReferences(step.diff.After, state)
  -> copied as-is                   2. provider.Apply(diff)
  -> "${...}" left as strings       3. state.Resources[name] = resource
  -> plan shows the template        4. next step can see previous outputs
```

- Plan output shows `${...}` as intent, making it clear what will be wired together.
- Resolution happens just-in-time when data is available.
- Dependencies guarantee ordering — referencing `webserver-ip` requires `dependsOn: [webserver-ip]`.
- The planner validates that referenced components exist and are listed in `dependsOn`.

## Resource Types

### IP (`type: ip`)

Reserves an IP address from an IPAM system.

#### Properties

| Property   | Type   | Required | Default | Description                          |
|------------|--------|----------|---------|--------------------------------------|
| `pool`     | string | yes      |         | IPAM pool or subnet name             |
| `version`  | string | no       | `4`     | IP version: `4` or `6`              |
| `attachTo` | string | no       |         | Resource to bind the IP to           |

#### Outputs

| Output    | Type   | Description                              |
|-----------|--------|------------------------------------------|
| `address` | string | The reserved IP address                  |
| `cidr`    | string | Address with prefix, e.g. `10.0.1.42/24` |
| `pool`    | string | Pool it was allocated from               |

### DNS (`type: dns`)

Creates or updates a DNS record.

#### Properties

| Property | Type   | Required | Default | Description                          |
|----------|--------|----------|---------|--------------------------------------|
| `zone`   | string | yes      |         | DNS zone (e.g. `example.com`)        |
| `record` | string | yes      |         | FQDN (e.g. `web.example.com`)       |
| `type`   | string | yes      |         | Record type: `A`, `AAAA`, `CNAME`   |
| `value`  | string | yes      |         | Record value (IP or hostname)        |
| `ttl`    | number | no       | `300`   | TTL in seconds                       |

#### Outputs

| Output  | Type   | Description          |
|---------|--------|----------------------|
| `fqdn`  | string | The created FQDN     |
| `type`  | string | Record type          |
| `value` | string | Resolved value       |

## Provider Implementations

### IP Providers

| Provider   | Backend          | Description                                      |
|------------|------------------|--------------------------------------------------|
| `netbox`   | NetBox IPAM      | Allocates IPs via NetBox REST API                |
| `phpipam`  | phpIPAM          | Allocates IPs via phpIPAM API                    |
| `static`   | Config-based     | Simple pool from environment config, no external system |

### DNS Providers

| Provider     | Backend        | Description                        |
|--------------|----------------|------------------------------------|
| `route53`    | AWS Route 53   | Manages records via AWS API        |
| `cloudflare` | Cloudflare    | Manages records via Cloudflare API |
| `powerdns`   | PowerDNS       | Manages records via PowerDNS API   |

Each provider implements the standard `types.Provider` interface with `Capabilities()` returning `[ip]` or `[dns]`.

```
pkg/provider/
  ipam/
    netbox.go
    phpipam.go
    static.go
  dns/
    route53.go
    cloudflare.go
    powerdns.go
```

## Environment Configuration

### IPAM Environment

```yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: corp-ipam
  labels:
    function: ipam
spec:
  provider: netbox
  config:
    url: https://netbox.internal
    token: ${NETBOX_TOKEN}
```

### DNS Environment

```yaml
apiVersion: dcm.io/v1
kind: Environment
metadata:
  name: corp-dns
  labels:
    function: dns
spec:
  provider: cloudflare
  config:
    apiToken: ${CF_API_TOKEN}
    zoneId: abc123
```

## Policy Routing

Policies route IP and DNS components to the correct environments:

```yaml
apiVersion: dcm.io/v1
kind: Policy
metadata:
  name: network-routing
spec:
  rules:
    - name: ip-to-netbox
      match:
        types: [ip]
      environments:
        required: corp-ipam

    - name: dns-to-cloudflare
      match:
        types: [dns]
      environments:
        required: corp-dns
```

## Full Example

An application that deploys a VM, reserves an IP, and creates a DNS record:

```yaml
apiVersion: dcm.io/v1
kind: Application
metadata:
  name: webapp
  labels:
    team: platform
spec:
  components:
    - name: server
      type: vm
      labels:
        tier: production
      properties:
        image: quay.io/containerdisks/ubuntu:22.04
        cpu: 4
        memory: 8Gi
        userData: |
          #cloud-config
          packages: [nginx]

    - name: server-ip
      type: ip
      dependsOn: [server]
      properties:
        pool: production-vlan100
        attachTo: ${server.outputs.vmName}

    - name: server-dns
      type: dns
      dependsOn: [server-ip]
      properties:
        zone: example.com
        record: webapp.example.com
        type: A
        value: ${server-ip.outputs.address}
        ttl: 300
```

Deployment flow:

1. Scheduler routes `server` (type `vm`) to a KubeVirt environment via policies.
2. Scheduler routes `server-ip` (type `ip`) to the NetBox environment.
3. Scheduler routes `server-dns` (type `dns`) to the Cloudflare environment.
4. Executor processes in order: `server` -> `server-ip` -> `server-dns`.
5. At each step, `${...}` references are resolved from the outputs of previously completed steps.
6. On destroy, resources are removed in reverse order: DNS record -> IP release -> VM deletion.

## Implementation Order

1. **Output references in executor** — Resolve `${component.outputs.field}` in properties before calling `Apply`. Add reference validation in planner to check that referenced components exist and are in `dependsOn`.
2. **Resource type constants** — Add `ResourceTypeIP` and `ResourceTypeDNS` to `types/provider.go`, add type schemas to `api/types.go`.
3. **Mock support** — Add `ip` and `dns` to the mock provider for local testing.
4. **Static IP provider** — Simple pool-based IPAM that allocates from a configured CIDR range. Good starting point before integrating external IPAM systems.
5. **DNS provider** — Start with CloudFlare or PowerDNS (simplest APIs).
6. **External IPAM** — NetBox or phpIPAM integration.
