package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dcm-io/dcm/pkg/types"
)

// Provider manages DNS records via the PowerDNS Authoritative Server API.
type Provider struct {
	apiURL string
	apiKey string
	server string
	client *http.Client
}

// Config holds the configuration for the PowerDNS provider.
type Config struct {
	APIURL string // e.g. "http://localhost:8081"
	APIKey string // X-API-Key header value
	Server string // PowerDNS server ID (default: "localhost")
}

// New creates a new PowerDNS provider.
func New(cfg Config) (*Provider, error) {
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("powerdns: apiUrl is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("powerdns: apiKey is required")
	}
	server := cfg.Server
	if server == "" {
		server = "localhost"
	}
	return &Provider{
		apiURL: strings.TrimRight(cfg.APIURL, "/"),
		apiKey: cfg.APIKey,
		server: server,
		client: &http.Client{},
	}, nil
}

func (p *Provider) Name() string {
	return "powerdns"
}

func (p *Provider) Capabilities() []types.ResourceType {
	return []types.ResourceType{types.ResourceTypeDNS}
}

func (p *Provider) Plan(desired, current *types.Resource) (*types.Diff, error) {
	if current == nil {
		return &types.Diff{
			Action:   types.DiffActionCreate,
			Resource: desired.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			After:    desired.Properties,
		}, nil
	}

	if !propsEqual(desired.Properties, current.Properties) {
		return &types.Diff{
			Action:   types.DiffActionUpdate,
			Resource: current.Name,
			Type:     desired.Type,
			Provider: p.Name(),
			Before:   current.Properties,
			After:    desired.Properties,
		}, nil
	}

	return &types.Diff{
		Action:   types.DiffActionNone,
		Resource: current.Name,
		Type:     desired.Type,
		Provider: p.Name(),
	}, nil
}

func (p *Provider) Apply(diff *types.Diff) (*types.Resource, error) {
	props := diff.After
	zone, _ := props["zone"].(string)
	record, _ := props["record"].(string)
	recType, _ := props["type"].(string)
	value, _ := props["value"].(string)

	if zone == "" || record == "" || recType == "" || value == "" {
		return nil, fmt.Errorf("dns resource %q requires zone, record, type, and value", diff.Resource)
	}

	ttl := 300
	if t, ok := props["ttl"]; ok {
		switch v := t.(type) {
		case float64:
			ttl = int(v)
		case int:
			ttl = v
		case json.Number:
			n, _ := v.Int64()
			ttl = int(n)
		}
	}

	fqdn := ensureTrailingDot(record)
	zoneFQDN := ensureTrailingDot(zone)

	// PATCH the zone to create/update the record.
	payload := rrsetPatch{
		RRSets: []rrset{{
			Name:       fqdn,
			Type:       strings.ToUpper(recType),
			TTL:        ttl,
			ChangeType: "REPLACE",
			Records: []rrRecord{{
				Content:  value,
				Disabled: false,
			}},
		}},
	}

	if err := p.patchZone(zoneFQDN, payload); err != nil {
		return nil, fmt.Errorf("creating DNS record %s: %w", record, err)
	}

	return &types.Resource{
		Name:       diff.Resource,
		Type:       diff.Type,
		Provider:   p.Name(),
		Properties: props,
		Outputs: map[string]any{
			"fqdn":  strings.TrimRight(fqdn, "."),
			"type":  strings.ToUpper(recType),
			"value": value,
			"ttl":   ttl,
		},
		Status: types.ResourceStatusReady,
	}, nil
}

func (p *Provider) Destroy(resource *types.Resource) error {
	zone, _ := resource.Properties["zone"].(string)
	record, _ := resource.Properties["record"].(string)
	recType, _ := resource.Properties["type"].(string)

	if zone == "" || record == "" || recType == "" {
		return nil
	}

	payload := rrsetPatch{
		RRSets: []rrset{{
			Name:       ensureTrailingDot(record),
			Type:       strings.ToUpper(recType),
			ChangeType: "DELETE",
		}},
	}

	return p.patchZone(ensureTrailingDot(zone), payload)
}

func (p *Provider) Status(resource *types.Resource) (types.ResourceStatus, error) {
	zone, _ := resource.Properties["zone"].(string)
	record, _ := resource.Properties["record"].(string)
	recType, _ := resource.Properties["type"].(string)

	if zone == "" || record == "" {
		return types.ResourceStatusUnknown, nil
	}

	fqdn := ensureTrailingDot(record)
	zoneFQDN := ensureTrailingDot(zone)

	url := fmt.Sprintf("%s/api/v1/servers/%s/zones/%s", p.apiURL, p.server, zoneFQDN)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return types.ResourceStatusUnknown, err
	}
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return types.ResourceStatusUnknown, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return types.ResourceStatusUnknown, nil
	}

	var zoneData struct {
		RRSets []rrset `json:"rrsets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&zoneData); err != nil {
		return types.ResourceStatusUnknown, err
	}

	wantType := strings.ToUpper(recType)
	for _, rr := range zoneData.RRSets {
		if rr.Name == fqdn && rr.Type == wantType {
			return types.ResourceStatusReady, nil
		}
	}

	return types.ResourceStatusUnknown, nil
}

// PowerDNS API types.
type rrsetPatch struct {
	RRSets []rrset `json:"rrsets"`
}

type rrset struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	TTL        int        `json:"ttl,omitempty"`
	ChangeType string     `json:"changetype"`
	Records    []rrRecord `json:"records,omitempty"`
}

type rrRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

func (p *Provider) patchZone(zoneFQDN string, payload rrsetPatch) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/servers/%s/zones/%s", p.apiURL, p.server, zoneFQDN)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PowerDNS API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func ensureTrailingDot(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

func propsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if fmt.Sprintf("%v", b[k]) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}
