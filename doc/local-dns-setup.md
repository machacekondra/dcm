# Local DNS Setup (macOS)

This guide sets up a local DNS stack so that domains managed by DCM (e.g., `*.example.ondra`) resolve on your Mac. The stack uses:

- **PowerDNS** — authoritative DNS server (runs in a Podman container)
- **dnsmasq** — lightweight DNS forwarder that routes queries for your zone to PowerDNS
- **/etc/resolver** — macOS per-domain resolver config that tells the OS to use dnsmasq

```
Browser / curl / ping
        │
        ▼
/etc/resolver/example.ondra  →  dnsmasq (127.0.0.1:53)
                                    │
                                    ▼
                              PowerDNS (127.0.0.1:5354)
                                    │
                                    ▼
                              Zone: example.ondra
```

## 1. Start PowerDNS

```bash
./hack/setup-powerdns.sh
```

This starts a PowerDNS container listening on:
- **API**: `http://localhost:8081` (key: `dcm-secret`)
- **DNS**: `localhost:5354` (UDP/TCP)
- **Zone**: `example.ondra`

Verify it's running:

```bash
./hack/setup-powerdns.sh status
```

Test DNS directly:

```bash
dig @localhost -p 5354 ns1.example.ondra
```

## 2. Install and configure dnsmasq

### Install

```bash
brew install dnsmasq
```

### Configure

Edit the dnsmasq config to forward `example.ondra` queries to PowerDNS:

```bash
echo 'server=/example.ondra/127.0.0.1#5354' >> $(brew --prefix)/etc/dnsmasq.conf
```

This tells dnsmasq: "for anything under `example.ondra`, ask PowerDNS on port 5354."

### Start dnsmasq

```bash
sudo brew services start dnsmasq
```

dnsmasq will listen on `127.0.0.1:53`.

If dnsmasq is already running, restart it to pick up the config change:

```bash
sudo brew services restart dnsmasq
```

### Verify dnsmasq is forwarding

```bash
dig @127.0.0.1 ns1.example.ondra
```

You should see `127.0.0.1` in the answer.

## 3. Configure macOS resolver

Create a resolver file so macOS sends all `*.example.ondra` queries to dnsmasq:

```bash
sudo mkdir -p /etc/resolver
sudo bash -c 'echo "nameserver 127.0.0.1" > /etc/resolver/example.ondra'
```

This tells macOS: "for any domain ending in `.example.ondra`, use `127.0.0.1` (dnsmasq) instead of your normal DNS."

### Verify

```bash
# Check macOS sees the resolver
scutil --dns | grep example.ondra

# Test end-to-end resolution
ping -c 1 ns1.example.ondra
```

## Full verification

Once all three pieces are in place, create a test record through DCM and resolve it:

```bash
# Create a record via PowerDNS API
curl -s -X PATCH \
  -H "X-API-Key: dcm-secret" \
  -H "Content-Type: application/json" \
  -d '{
    "rrsets": [{
      "name": "myapp.example.ondra.",
      "type": "A",
      "ttl": 300,
      "changetype": "REPLACE",
      "records": [{"content": "192.168.1.100", "disabled": false}]
    }]
  }' \
  "http://localhost:8081/api/v1/servers/localhost/zones/example.ondra."

# Resolve it — should return 192.168.1.100
dig myapp.example.ondra +short
ping -c 1 myapp.example.ondra
```

## Teardown

```bash
# Stop PowerDNS
./hack/setup-powerdns.sh stop

# Stop dnsmasq
sudo brew services stop dnsmasq

# Remove resolver config
sudo rm /etc/resolver/example.ondra
```

## Troubleshooting

**dnsmasq won't start (port 53 in use)**

Check what's using port 53:

```bash
sudo lsof -i :53
```

If it's `mDNSResponder`, dnsmasq should still work — macOS uses `/etc/resolver` to route specific domains, so both can coexist.

**dig works but ping doesn't resolve**

macOS caches DNS. Flush the cache:

```bash
sudo dscacheutil -flushcache
sudo killall -HUP mDNSResponder
```

**Changes to PowerDNS records don't appear**

dnsmasq caches responses. Either wait for the TTL to expire or restart dnsmasq:

```bash
sudo brew services restart dnsmasq
```
