#!/usr/bin/env bash
#
# Sets up a local PowerDNS authoritative server for testing DCM DNS management.
#
# Usage:
#   ./hack/setup-powerdns.sh          # start PowerDNS
#   ./hack/setup-powerdns.sh stop     # stop and remove
#   ./hack/setup-powerdns.sh status   # check status
#   ./hack/setup-powerdns.sh test     # create a test record and query it
#
# PowerDNS API: http://localhost:8081
# DNS queries:  dig @localhost -p 5354 petclinic.example.com
# DoH (Firefox): https://localhost:8443/dns-query
# API key:      dcm-secret
#

set -euo pipefail

CONTAINER_NAME="dcm-powerdns"
DOH_CONTAINER_NAME="dcm-powerdns-doh"
API_PORT="8081"
DNS_PORT="5354"
DOH_PORT="8443"
API_KEY="dcm-secret"
ZONE="example.com"
CERT_DIR="${HOME}/.dcm/certs"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[-]${NC} $*"; }

check_podman() {
    if ! command -v podman &>/dev/null; then
        error "Podman is not installed. Please install Podman first."
        exit 1
    fi
    if ! podman info &>/dev/null; then
        error "Podman is not running. Please start Podman."
        exit 1
    fi
}

start() {
    check_podman

    if podman ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        warn "PowerDNS is already running."
        status
        return
    fi

    # Remove stopped container if exists.
    podman rm -f "$CONTAINER_NAME" &>/dev/null || true

    info "Starting PowerDNS authoritative server..."
    podman run -d \
        --name "$CONTAINER_NAME" \
        -p "${API_PORT}:8081" \
        -p "${DNS_PORT}:5354/udp" \
        -p "${DNS_PORT}:5354/tcp" \
        -e PDNS_AUTH_API_KEY="$API_KEY" \
        powerdns/pdns-auth-master \
        --api=yes \
        --api-key="$API_KEY" \
        --webserver=yes \
        --webserver-address=0.0.0.0 \
        --webserver-port=8081 \
        --webserver-allow-from=0.0.0.0/0 \
        --launch=gsqlite3 \
        --gsqlite3-database=/var/lib/powerdns/pdns.sqlite3 \
        --local-address=0.0.0.0 \
        --local-port=5354 \
        --log-dns-queries=yes \
        --loglevel=6 \
        > /dev/null

    info "Waiting for PowerDNS to start..."
    for i in $(seq 1 15); do
        if curl -s -o /dev/null -w "%{http_code}" \
            -H "X-API-Key: $API_KEY" \
            "http://localhost:${API_PORT}/api/v1/servers/localhost" 2>/dev/null | grep -q "200"; then
            break
        fi
        sleep 1
    done

    # Check if API is responding.
    if ! curl -s -H "X-API-Key: $API_KEY" "http://localhost:${API_PORT}/api/v1/servers/localhost" &>/dev/null; then
        error "PowerDNS API not responding. Check logs: podman logs $CONTAINER_NAME"
        exit 1
    fi

    info "Creating zone: ${ZONE}"
    curl -s -X POST \
        -H "X-API-Key: $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"name\": \"${ZONE}.\",
            \"kind\": \"Native\",
            \"nameservers\": [\"ns1.${ZONE}.\"],
            \"rrsets\": [
                {
                    \"name\": \"ns1.${ZONE}.\",
                    \"type\": \"A\",
                    \"ttl\": 3600,
                    \"changetype\": \"REPLACE\",
                    \"records\": [{\"content\": \"127.0.0.1\", \"disabled\": false}]
                }
            ]
        }" \
        "http://localhost:${API_PORT}/api/v1/servers/localhost/zones" > /dev/null 2>&1 || true

    start_doh

    echo ""
    info "PowerDNS is ready!"
    echo ""
    echo "  API endpoint:  http://localhost:${API_PORT}"
    echo "  API key:       ${API_KEY}"
    echo "  DNS port:      ${DNS_PORT}"
    echo "  DoH endpoint:  https://localhost:${DOH_PORT}/dns-query"
    echo "  Zone:          ${ZONE}"
    echo ""
    echo "  Test with:"
    echo "    curl -s -H 'X-API-Key: ${API_KEY}' http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}. | jq .rrsets"
    echo ""
    echo "  Firefox DoH setup:"
    echo "    1. Open https://localhost:${DOH_PORT} in Firefox and accept the self-signed cert"
    echo "    2. Settings -> Privacy & Security -> DNS over HTTPS -> Max Protection"
    echo "    3. Custom provider: https://localhost:${DOH_PORT}/dns-query"
    echo ""
    echo "  DCM environment config:"
    echo "    Name:     powerdns"
    echo "    Provider: powerdns"
    echo "    Config:"
    echo "      apiUrl:  http://localhost:${API_PORT}"
    echo "      apiKey:  ${API_KEY}"
    echo "      server:  localhost"
    echo ""
}

generate_certs() {
    if [ -f "${CERT_DIR}/cert.pem" ] && [ -f "${CERT_DIR}/key.pem" ]; then
        return
    fi

    info "Generating self-signed TLS certificate for DoH..."
    mkdir -p "$CERT_DIR"
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -days 3650 -nodes \
        -keyout "${CERT_DIR}/key.pem" \
        -out "${CERT_DIR}/cert.pem" \
        -subj "/CN=localhost" \
        -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
        2>/dev/null
    info "Certificate stored in ${CERT_DIR}"
}

start_doh() {
    if podman ps --format '{{.Names}}' | grep -q "^${DOH_CONTAINER_NAME}$"; then
        warn "DoH proxy is already running."
        return
    fi

    podman rm -f "$DOH_CONTAINER_NAME" &>/dev/null || true

    generate_certs

    # Get the PowerDNS container IP so dnsdist can forward to it directly.
    local pdns_ip
    pdns_ip=$(podman inspect "$CONTAINER_NAME" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null)
    if [ -z "$pdns_ip" ]; then
        error "Could not determine PowerDNS container IP. Is it running?"
        return 1
    fi

    # Write dnsdist config that forwards to PowerDNS and serves DoH.
    local config_dir
    config_dir=$(mktemp -d)
    cat > "${config_dir}/dnsdist.conf" <<DNSDIST_EOF
-- Forward all queries to the PowerDNS auth server.
newServer({address="${pdns_ip}:5354", checkInterval=1, checkName="example.com."})

-- Serve DNS-over-HTTPS on a non-privileged port inside the container.
addDOHLocal("0.0.0.0:8443", "/etc/dnsdist/cert.pem", "/etc/dnsdist/key.pem", "/dns-query")
DNSDIST_EOF

    info "Starting dnsdist DoH proxy..."
    podman run -d \
        --name "$DOH_CONTAINER_NAME" \
        -p "${DOH_PORT}:8443/tcp" \
        -v "${CERT_DIR}/cert.pem:/etc/dnsdist/cert.pem:ro,z" \
        -v "${CERT_DIR}/key.pem:/etc/dnsdist/key.pem:ro,z" \
        -v "${config_dir}/dnsdist.conf:/etc/dnsdist/dnsdist.conf:ro,z" \
        powerdns/dnsdist-master \
        > /dev/null

    # Wait for DoH to be ready.
    for i in $(seq 1 10); do
        if curl -sk -o /dev/null -w "%{http_code}" \
            "https://localhost:${DOH_PORT}/dns-query" 2>/dev/null | grep -qE "200|400"; then
            break
        fi
        sleep 1
    done

    info "DoH proxy is ready on https://localhost:${DOH_PORT}/dns-query"
}

stop() {
    check_podman

    if podman ps -a --format '{{.Names}}' | grep -q "^${DOH_CONTAINER_NAME}$"; then
        info "Stopping DoH proxy..."
        podman rm -f "$DOH_CONTAINER_NAME" > /dev/null
        info "DoH proxy stopped and removed."
    fi

    if podman ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        info "Stopping PowerDNS..."
        podman rm -f "$CONTAINER_NAME" > /dev/null
        info "PowerDNS stopped and removed."
    else
        warn "PowerDNS container not found."
    fi
}

status() {
    check_podman
    if podman ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        info "PowerDNS is running."
        echo ""
        echo "  Container: $(podman ps --filter name=$CONTAINER_NAME --format '{{.ID}}  {{.Status}}  ports={{.Ports}}')"
        echo ""

        # Show zones.
        zones=$(curl -s -H "X-API-Key: $API_KEY" "http://localhost:${API_PORT}/api/v1/servers/localhost/zones" 2>/dev/null)
        if [ -n "$zones" ]; then
            echo "  Zones:"
            echo "$zones" | python3 -c "
import sys, json
for z in json.load(sys.stdin):
    print(f\"    - {z['name']} ({z['kind']}, serial={z.get('serial', 'N/A')})\")" 2>/dev/null || echo "    (could not parse zones)"
        fi
        echo ""

        # Show DoH proxy status.
        if podman ps --format '{{.Names}}' | grep -q "^${DOH_CONTAINER_NAME}$"; then
            info "DoH proxy is running."
            echo "  Container: $(podman ps --filter name=$DOH_CONTAINER_NAME --format '{{.ID}}  {{.Status}}  ports={{.Ports}}')"
            echo "  DoH URL:   https://localhost:${DOH_PORT}/dns-query"
        else
            warn "DoH proxy is not running."
        fi
        echo ""
    else
        warn "PowerDNS is not running."
    fi
}

test_record() {
    check_podman
    if ! podman ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        error "PowerDNS is not running. Start it first: $0"
        exit 1
    fi

    TEST_RECORD="test.${ZONE}"
    TEST_IP="192.168.1.42"

    info "Creating A record: ${TEST_RECORD} -> ${TEST_IP}"
    curl -s -X PATCH \
        -H "X-API-Key: $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"rrsets\": [{
                \"name\": \"${TEST_RECORD}.\",
                \"type\": \"A\",
                \"ttl\": 300,
                \"changetype\": \"REPLACE\",
                \"records\": [{\"content\": \"${TEST_IP}\", \"disabled\": false}]
            }]
        }" \
        "http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}."
    echo ""

    info "Querying DNS record..."
    if command -v dig &>/dev/null; then
        dig @localhost -p "$DNS_PORT" "$TEST_RECORD" A +short
    else
        warn "dig not found, querying via API instead:"
        curl -s -H "X-API-Key: $API_KEY" \
            "http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}." \
            | python3 -c "
import sys, json
data = json.load(sys.stdin)
for rr in data.get('rrsets', []):
    if rr['name'] == '${TEST_RECORD}.':
        for r in rr['records']:
            print(f\"  {rr['name']}  {rr['ttl']}  {rr['type']}  {r['content']}\")" 2>/dev/null
    fi

    echo ""
    info "Cleaning up test record..."
    curl -s -X PATCH \
        -H "X-API-Key: $API_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"rrsets\": [{
                \"name\": \"${TEST_RECORD}.\",
                \"type\": \"A\",
                \"changetype\": \"DELETE\"
            }]
        }" \
        "http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}."
    echo ""

    info "Test complete."
}

test_doh() {
    check_podman
    if ! podman ps --format '{{.Names}}' | grep -q "^${DOH_CONTAINER_NAME}$"; then
        error "DoH proxy is not running. Start it first: $0"
        exit 1
    fi

    local query_name="${1:-ns1.example.com}"
    info "Querying ${query_name} via DoH (https://localhost:${DOH_PORT}/dns-query)..."

    # Build a DNS wire-format query using python and send via curl POST.
    local response
    response=$(curl -sk --http2 -X POST \
        -H "Content-Type: application/dns-message" \
        -H "Accept: application/dns-message" \
        --data-binary @<(python3 -c "
import struct, sys
qname = b''
for label in '${query_name}'.split('.'):
    qname += bytes([len(label)]) + label.encode()
qname += b'\x00'
header = struct.pack('>HHHHHH', 0, 0x0100, 1, 0, 0, 0)
sys.stdout.buffer.write(header + qname + struct.pack('>HH', 1, 1))
") \
        -o /tmp/dcm-doh-response.bin \
        -w "%{http_code}" \
        "https://localhost:${DOH_PORT}/dns-query")

    if [ "$response" != "200" ]; then
        error "DoH query failed (HTTP ${response})"
        return 1
    fi

    python3 -c "
import struct
with open('/tmp/dcm-doh-response.bin', 'rb') as f:
    data = f.read()
if len(data) < 12:
    print('  Empty response')
    exit()
rcode = data[3] & 0xf
ancount = struct.unpack('>H', data[6:8])[0]
print(f'  Status: NOERROR' if rcode == 0 else f'  Status: RCODE={rcode}')
print(f'  Answers: {ancount}')
if ancount > 0:
    pos = 12
    while data[pos] != 0:
        pos += data[pos] + 1
    pos += 5
    for i in range(ancount):
        pos += 2
        rtype = struct.unpack('>H', data[pos:pos+2])[0]
        pos += 2 + 2
        ttl = struct.unpack('>I', data[pos:pos+4])[0]
        pos += 4
        rdlen = struct.unpack('>H', data[pos:pos+2])[0]
        pos += 2
        if rtype == 1 and rdlen == 4:
            ip = '.'.join(str(b) for b in data[pos:pos+rdlen])
            print(f'  ${query_name} {ttl} IN A {ip}')
        else:
            print(f'  ${query_name} {ttl} IN type={rtype} rdlen={rdlen}')
        pos += rdlen
"
    info "DoH test complete."
}

case "${1:-start}" in
    start)    start ;;
    stop)     stop ;;
    status)   status ;;
    test)     test_record ;;
    test-doh) test_doh "${2:-}" ;;
    *)
        echo "Usage: $0 {start|stop|status|test|test-doh [name]}"
        exit 1
        ;;
esac
