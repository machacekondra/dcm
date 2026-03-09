package dns

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func newTestServer(t *testing.T) (*httptest.Server, *Provider) {
	t.Helper()

	// In-memory zone data.
	zones := map[string][]rrset{
		"example.com.": {},
	}

	mux := http.NewServeMux()

	// GET zone — return rrsets.
	mux.HandleFunc("GET /api/v1/servers/localhost/zones/{zone}", func(w http.ResponseWriter, r *http.Request) {
		zone := r.PathValue("zone")
		rrsets, ok := zones[zone]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"rrsets": rrsets})
	})

	// PATCH zone — apply rrset changes.
	mux.HandleFunc("PATCH /api/v1/servers/localhost/zones/{zone}", func(w http.ResponseWriter, r *http.Request) {
		zone := r.PathValue("zone")
		if _, ok := zones[zone]; !ok {
			http.NotFound(w, r)
			return
		}

		var patch rrsetPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		for _, rr := range patch.RRSets {
			if rr.ChangeType == "DELETE" {
				// Remove matching rrsets.
				filtered := zones[zone][:0]
				for _, existing := range zones[zone] {
					if !(existing.Name == rr.Name && existing.Type == rr.Type) {
						filtered = append(filtered, existing)
					}
				}
				zones[zone] = filtered
			} else {
				// REPLACE: remove old, add new.
				filtered := zones[zone][:0]
				for _, existing := range zones[zone] {
					if !(existing.Name == rr.Name && existing.Type == rr.Type) {
						filtered = append(filtered, existing)
					}
				}
				filtered = append(filtered, rr)
				zones[zone] = filtered
			}
		}

		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)

	p, err := New(Config{
		APIURL: ts.URL,
		APIKey: "test-key",
		Server: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}

	return ts, p
}

func TestProvider_Name(t *testing.T) {
	_, p := newTestServer(t)
	if p.Name() != "powerdns" {
		t.Errorf("expected 'powerdns', got %q", p.Name())
	}
}

func TestProvider_Capabilities(t *testing.T) {
	_, p := newTestServer(t)
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != types.ResourceTypeDNS {
		t.Errorf("expected [dns], got %v", caps)
	}
}

func TestProvider_PlanCreate(t *testing.T) {
	_, p := newTestServer(t)

	desired := &types.Resource{
		Name: "app-dns",
		Type: types.ResourceTypeDNS,
		Properties: map[string]any{
			"zone":   "example.com",
			"record": "app.example.com",
			"type":   "A",
			"value":  "10.0.1.5",
			"ttl":    300,
		},
	}

	diff, err := p.Plan(desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionCreate {
		t.Errorf("expected create, got %s", diff.Action)
	}
	if diff.Provider != "powerdns" {
		t.Errorf("expected provider 'powerdns', got %q", diff.Provider)
	}
}

func TestProvider_PlanNoChange(t *testing.T) {
	_, p := newTestServer(t)

	resource := &types.Resource{
		Name: "app-dns",
		Type: types.ResourceTypeDNS,
		Properties: map[string]any{
			"zone":   "example.com",
			"record": "app.example.com",
			"type":   "A",
			"value":  "10.0.1.5",
		},
	}

	diff, err := p.Plan(resource, resource)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionNone {
		t.Errorf("expected none, got %s", diff.Action)
	}
}

func TestProvider_PlanUpdate(t *testing.T) {
	_, p := newTestServer(t)

	current := &types.Resource{
		Name: "app-dns",
		Type: types.ResourceTypeDNS,
		Properties: map[string]any{
			"zone":   "example.com",
			"record": "app.example.com",
			"type":   "A",
			"value":  "10.0.1.5",
		},
	}
	desired := &types.Resource{
		Name: "app-dns",
		Type: types.ResourceTypeDNS,
		Properties: map[string]any{
			"zone":   "example.com",
			"record": "app.example.com",
			"type":   "A",
			"value":  "10.0.1.99",
		},
	}

	diff, err := p.Plan(desired, current)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Action != types.DiffActionUpdate {
		t.Errorf("expected update, got %s", diff.Action)
	}
}

func TestProvider_ApplyAndStatus(t *testing.T) {
	ts, p := newTestServer(t)
	defer ts.Close()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "app-dns",
		Type:     types.ResourceTypeDNS,
		Provider: "powerdns",
		After: map[string]any{
			"zone":   "example.com",
			"record": "app.example.com",
			"type":   "A",
			"value":  "10.0.1.5",
			"ttl":    300,
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	if resource.Status != types.ResourceStatusReady {
		t.Errorf("expected ready, got %s", resource.Status)
	}
	if resource.Outputs["fqdn"] != "app.example.com" {
		t.Errorf("expected fqdn 'app.example.com', got %v", resource.Outputs["fqdn"])
	}
	if resource.Outputs["type"] != "A" {
		t.Errorf("expected type 'A', got %v", resource.Outputs["type"])
	}
	if resource.Outputs["value"] != "10.0.1.5" {
		t.Errorf("expected value '10.0.1.5', got %v", resource.Outputs["value"])
	}

	// Check status — should find the record.
	status, err := p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusReady {
		t.Errorf("expected ready status, got %s", status)
	}
}

func TestProvider_ApplyAndDestroy(t *testing.T) {
	ts, p := newTestServer(t)
	defer ts.Close()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "app-dns",
		Type:     types.ResourceTypeDNS,
		Provider: "powerdns",
		After: map[string]any{
			"zone":   "example.com",
			"record": "web.example.com",
			"type":   "A",
			"value":  "192.168.1.1",
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	// Verify it exists.
	status, err := p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusReady {
		t.Errorf("expected ready, got %s", status)
	}

	// Destroy it.
	if err := p.Destroy(resource); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone.
	status, err = p.Status(resource)
	if err != nil {
		t.Fatal(err)
	}
	if status != types.ResourceStatusUnknown {
		t.Errorf("expected unknown after destroy, got %s", status)
	}
}

func TestProvider_ApplyMissingFields(t *testing.T) {
	ts, p := newTestServer(t)
	defer ts.Close()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "bad-dns",
		Type:     types.ResourceTypeDNS,
		Provider: "powerdns",
		After:    map[string]any{"zone": "example.com"},
	}

	_, err := p.Apply(diff)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestProvider_CNAME(t *testing.T) {
	ts, p := newTestServer(t)
	defer ts.Close()

	diff := &types.Diff{
		Action:   types.DiffActionCreate,
		Resource: "alias-dns",
		Type:     types.ResourceTypeDNS,
		Provider: "powerdns",
		After: map[string]any{
			"zone":   "example.com",
			"record": "www.example.com",
			"type":   "CNAME",
			"value":  "app.example.com",
			"ttl":    600,
		},
	}

	resource, err := p.Apply(diff)
	if err != nil {
		t.Fatal(err)
	}

	if resource.Outputs["type"] != "CNAME" {
		t.Errorf("expected CNAME, got %v", resource.Outputs["type"])
	}
	if resource.Outputs["ttl"] != 600 {
		t.Errorf("expected ttl 600, got %v", resource.Outputs["ttl"])
	}
}

func TestNew_MissingConfig(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for missing apiUrl")
	}

	_, err = New(Config{APIURL: "http://localhost:8081"})
	if err == nil {
		t.Fatal("expected error for missing apiKey")
	}
}

func TestProvider_DefaultServer(t *testing.T) {
	p, err := New(Config{
		APIURL: "http://localhost:8081",
		APIKey: "key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.server != "localhost" {
		t.Errorf("expected default server 'localhost', got %q", p.server)
	}
}
