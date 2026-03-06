package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dcm-io/dcm/pkg/provider"
	"github.com/dcm-io/dcm/pkg/provider/mock"
	"github.com/dcm-io/dcm/pkg/store"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	reg := provider.NewRegistry()
	reg.Register(mock.New())
	return NewServer(db, reg)
}

func doRequest(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func TestApplicationCRUD(t *testing.T) {
	s := setupTestServer(t)

	// Create
	w := doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name":   "test-app",
		"labels": map[string]string{"env": "test"},
		"components": []map[string]any{
			{"name": "db", "type": "postgres", "properties": map[string]any{"version": "16"}},
			{"name": "web", "type": "container", "dependsOn": []string{"db"}, "properties": map[string]any{"image": "nginx"}},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List
	w = doRequest(t, s, "GET", "/api/v1/applications", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var apps []store.ApplicationRecord
	json.Unmarshal(w.Body.Bytes(), &apps)
	if len(apps) != 1 {
		t.Fatalf("list: expected 1 app, got %d", len(apps))
	}

	// Get
	w = doRequest(t, s, "GET", "/api/v1/applications/test-app", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Update
	w = doRequest(t, s, "PUT", "/api/v1/applications/test-app", map[string]any{
		"labels": map[string]string{"env": "staging"},
		"components": []map[string]any{
			{"name": "db", "type": "postgres"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Validate
	w = doRequest(t, s, "POST", "/api/v1/applications/test-app/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("validate: expected 200, got %d", w.Code)
	}

	// Get not found
	w = doRequest(t, s, "GET", "/api/v1/applications/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get not found: expected 404, got %d", w.Code)
	}

	// Delete
	w = doRequest(t, s, "DELETE", "/api/v1/applications/test-app", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApplicationValidation(t *testing.T) {
	s := setupTestServer(t)

	// Missing name
	w := doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"components": []map[string]any{{"name": "db", "type": "postgres"}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing name, got %d", w.Code)
	}

	// Missing components
	w = doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name": "app",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing components, got %d", w.Code)
	}

	// Invalid DAG (unknown dependency)
	w = doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name": "app",
		"components": []map[string]any{
			{"name": "web", "type": "container", "dependsOn": []string{"nonexistent"}},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid DAG, got %d", w.Code)
	}
}

func TestPolicyCRUD(t *testing.T) {
	s := setupTestServer(t)

	// Create
	w := doRequest(t, s, "POST", "/api/v1/policies", map[string]any{
		"name": "test-policy",
		"rules": []map[string]any{
			{
				"name":  "prefer-mock",
				"match": map[string]any{"type": "container"},
				"providers": map[string]any{
					"preferred": []string{"mock"},
				},
			},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List
	w = doRequest(t, s, "GET", "/api/v1/policies", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}

	// Validate
	w = doRequest(t, s, "POST", "/api/v1/policies/test-policy/validate", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("validate: expected 200, got %d", w.Code)
	}

	// Delete
	w = doRequest(t, s, "DELETE", "/api/v1/policies/test-policy", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPolicyWithInvalidCEL(t *testing.T) {
	s := setupTestServer(t)

	w := doRequest(t, s, "POST", "/api/v1/policies", map[string]any{
		"name": "bad-cel",
		"rules": []map[string]any{
			{
				"match": map[string]any{"expression": "invalid %%% syntax"},
			},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid CEL, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeploymentLifecycle(t *testing.T) {
	s := setupTestServer(t)

	// Create an application first.
	doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name": "deploy-app",
		"components": []map[string]any{
			{"name": "db", "type": "postgres"},
			{"name": "web", "type": "container", "dependsOn": []string{"db"}, "properties": map[string]any{"image": "nginx"}},
		},
	})

	// Dry run.
	w := doRequest(t, s, "POST", "/api/v1/deployments", map[string]any{
		"application": "deploy-app",
		"dryRun":      true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("dry run: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dryDep store.DeploymentRecord
	json.Unmarshal(w.Body.Bytes(), &dryDep)
	if dryDep.Status != "planned" {
		t.Fatalf("dry run: expected status 'planned', got %q", dryDep.Status)
	}

	// Delete the dry run deployment so we can create a real one.
	// Mark it as failed first so it can be cleaned up.
	dryDep.Status = "destroyed"
	s.store.UpdateDeployment(&dryDep)

	// Real deployment (async).
	w = doRequest(t, s, "POST", "/api/v1/deployments", map[string]any{
		"application": "deploy-app",
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("deploy: expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var dep store.DeploymentRecord
	json.Unmarshal(w.Body.Bytes(), &dep)

	// Wait for background deployment to complete.
	var status string
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		w = doRequest(t, s, "GET", "/api/v1/deployments/"+dep.ID, nil)
		json.Unmarshal(w.Body.Bytes(), &dep)
		status = dep.Status
		if status == "ready" || status == "failed" {
			break
		}
	}

	if status != "ready" {
		t.Fatalf("deployment did not reach ready, got %q, error: %s", status, dep.Error)
	}

	// Check history.
	w = doRequest(t, s, "GET", "/api/v1/deployments/"+dep.ID+"/history", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("history: expected 200, got %d", w.Code)
	}
	var history []store.HistoryRecord
	json.Unmarshal(w.Body.Bytes(), &history)
	if len(history) < 3 {
		t.Fatalf("expected at least 3 history entries, got %d", len(history))
	}

	// Duplicate deployment should fail (one per app).
	w = doRequest(t, s, "POST", "/api/v1/deployments", map[string]any{
		"application": "deploy-app",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate deploy: expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// Destroy deployment.
	w = doRequest(t, s, "DELETE", "/api/v1/deployments/"+dep.ID, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("destroy: expected 202, got %d: %s", w.Code, w.Body.String())
	}

	// Wait for destroy.
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		w = doRequest(t, s, "GET", "/api/v1/deployments/"+dep.ID, nil)
		json.Unmarshal(w.Body.Bytes(), &dep)
		if dep.Status == "destroyed" {
			break
		}
	}
	if dep.Status != "destroyed" {
		t.Fatalf("expected destroyed, got %q", dep.Status)
	}

	// After destroy, application can be deleted.
	w = doRequest(t, s, "DELETE", "/api/v1/applications/deploy-app", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete app after destroy: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCannotDeleteAppWithActiveDeployment(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name":       "locked-app",
		"components": []map[string]any{{"name": "db", "type": "postgres"}},
	})

	doRequest(t, s, "POST", "/api/v1/deployments", map[string]any{
		"application": "locked-app",
	})

	// Wait for deployment to complete.
	time.Sleep(200 * time.Millisecond)

	w := doRequest(t, s, "DELETE", "/api/v1/applications/locked-app", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 (has active deployments), got %d: %s", w.Code, w.Body.String())
	}
}

func TestProvidersList(t *testing.T) {
	s := setupTestServer(t)

	w := doRequest(t, s, "GET", "/api/v1/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var providers []providerInfo
	json.Unmarshal(w.Body.Bytes(), &providers)
	if len(providers) != 1 || providers[0].Name != "mock" {
		t.Fatalf("expected [mock], got %v", providers)
	}
}

func TestDeploymentWithPolicies(t *testing.T) {
	s := setupTestServer(t)

	// Create app.
	doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name": "policy-app",
		"components": []map[string]any{
			{"name": "web", "type": "container", "properties": map[string]any{"image": "nginx"}},
		},
	})

	// Create policy.
	doRequest(t, s, "POST", "/api/v1/policies", map[string]any{
		"name": "prefer-mock",
		"rules": []map[string]any{{
			"match":     map[string]any{"type": "container"},
			"providers": map[string]any{"preferred": []string{"mock"}},
		}},
	})

	// Deploy with policy.
	w := doRequest(t, s, "POST", "/api/v1/deployments", map[string]any{
		"application": "policy-app",
		"policies":    []string{"prefer-mock"},
	})
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}

	var dep store.DeploymentRecord
	json.Unmarshal(w.Body.Bytes(), &dep)

	// Wait for completion.
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		w = doRequest(t, s, "GET", "/api/v1/deployments/"+dep.ID, nil)
		json.Unmarshal(w.Body.Bytes(), &dep)
		if dep.Status == "ready" || dep.Status == "failed" {
			break
		}
	}
	if dep.Status != "ready" {
		t.Fatalf("expected ready, got %q, error: %s", dep.Status, dep.Error)
	}
	if len(dep.Policies) != 1 || dep.Policies[0] != "prefer-mock" {
		t.Fatalf("expected policies [prefer-mock], got %v", dep.Policies)
	}
}

func TestEvaluatePolicies(t *testing.T) {
	s := setupTestServer(t)

	doRequest(t, s, "POST", "/api/v1/applications", map[string]any{
		"name": "eval-app",
		"components": []map[string]any{
			{"name": "db", "type": "postgres"},
			{"name": "web", "type": "container", "properties": map[string]any{"image": "nginx"}},
		},
	})

	doRequest(t, s, "POST", "/api/v1/policies", map[string]any{
		"name": "test-eval",
		"rules": []map[string]any{{
			"match":     map[string]any{"type": "postgres"},
			"providers": map[string]any{"preferred": []string{"mock"}},
		}},
	})

	w := doRequest(t, s, "POST", "/api/v1/policies/evaluate", map[string]any{
		"application": "eval-app",
		"policies":    []string{"test-eval"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var results []evaluateComponentResult
	json.Unmarshal(w.Body.Bytes(), &results)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}
