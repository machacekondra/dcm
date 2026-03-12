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
# DNS queries:  dig @localhost -p 5354 petclinic.example.ondra
# API key:      dcm-secret
#

set -euo pipefail

CONTAINER_NAME="dcm-powerdns"
API_PORT="8081"
DNS_PORT="5354"
API_KEY="dcm-secret"
ZONE="example.ondra"

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

    echo ""
    info "PowerDNS is ready!"
    echo ""
    echo "  API endpoint:  http://localhost:${API_PORT}"
    echo "  API key:       ${API_KEY}"
    echo "  DNS port:      ${DNS_PORT}"
    echo "  Zone:          ${ZONE}"
    echo ""
    echo "  Test with:"
    echo "    curl -s -H 'X-API-Key: ${API_KEY}' http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}. | jq .rrsets"
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

stop() {
    check_podman

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
    #curl -s -X PATCH \
    #    -H "X-API-Key: $API_KEY" \
    #    -H "Content-Type: application/json" \
    #    -d "{
    #        \"rrsets\": [{
    #            \"name\": \"${TEST_RECORD}.\",
    #            \"type\": \"A\",
    #            \"changetype\": \"DELETE\"
    #        }]
    #    }" \
    #    "http://localhost:${API_PORT}/api/v1/servers/localhost/zones/${ZONE}."
    #echo ""

    info "Test complete."
}

case "${1:-start}" in
    start)    start ;;
    stop)     stop ;;
    status)   status ;;
    test)     test_record ;;
    *)
        echo "Usage: $0 {start|stop|status|test}"
        exit 1
        ;;
esac
