package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dcm-io/dcm/pkg/compliance"
	"github.com/dcm-io/dcm/pkg/scheduler"
	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Server is the DCM API server.
type Server struct {
	store      *store.Store
	registry   *scheduler.Registry
	compliance *compliance.Engine
	mux        *http.ServeMux
}

// NewServer creates a new API server.
func NewServer(st *store.Store, registry *scheduler.Registry, comp *compliance.Engine) *Server {
	s := &Server{
		store:      st,
		registry:   registry,
		compliance: comp,
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Applications
	s.mux.HandleFunc("POST /api/v1/applications", s.handleCreateApplication)
	s.mux.HandleFunc("GET /api/v1/applications", s.handleListApplications)
	s.mux.HandleFunc("GET /api/v1/applications/{name}", s.handleGetApplication)
	s.mux.HandleFunc("PUT /api/v1/applications/{name}", s.handleUpdateApplication)
	s.mux.HandleFunc("DELETE /api/v1/applications/{name}", s.handleDeleteApplication)
	s.mux.HandleFunc("POST /api/v1/applications/{name}/validate", s.handleValidateApplication)

	// Policies
	s.mux.HandleFunc("POST /api/v1/policies", s.handleCreatePolicy)
	s.mux.HandleFunc("GET /api/v1/policies", s.handleListPolicies)
	s.mux.HandleFunc("GET /api/v1/policies/{name}", s.handleGetPolicy)
	s.mux.HandleFunc("PUT /api/v1/policies/{name}", s.handleUpdatePolicy)
	s.mux.HandleFunc("DELETE /api/v1/policies/{name}", s.handleDeletePolicy)
	s.mux.HandleFunc("POST /api/v1/policies/{name}/validate", s.handleValidatePolicy)
	s.mux.HandleFunc("POST /api/v1/policies/evaluate", s.handleEvaluatePolicies)

	// Deployments
	s.mux.HandleFunc("POST /api/v1/deployments", s.handleCreateDeployment)
	s.mux.HandleFunc("GET /api/v1/deployments", s.handleListDeployments)
	s.mux.HandleFunc("GET /api/v1/deployments/{id}", s.handleGetDeployment)
	s.mux.HandleFunc("DELETE /api/v1/deployments/{id}", s.handleDeleteDeployment)
	s.mux.HandleFunc("POST /api/v1/deployments/{id}/plan", s.handleDeploymentPlan)
	s.mux.HandleFunc("POST /api/v1/deployments/{id}/apply", s.handleDeploymentApply)
	s.mux.HandleFunc("GET /api/v1/deployments/{id}/history", s.handleDeploymentHistory)

	// Providers & Types
	s.mux.HandleFunc("GET /api/v1/providers", s.handleListProviders)
	s.mux.HandleFunc("GET /api/v1/types", s.handleListTypes)

	// Compliance
	s.mux.HandleFunc("POST /api/v1/compliance/check", s.handleComplianceCheck)

	// Environments
	s.mux.HandleFunc("POST /api/v1/environments", s.handleCreateEnvironment)
	s.mux.HandleFunc("GET /api/v1/environments", s.handleListEnvironments)
	s.mux.HandleFunc("GET /api/v1/environments/{name}", s.handleGetEnvironment)
	s.mux.HandleFunc("PUT /api/v1/environments/{name}", s.handleUpdateEnvironment)
	s.mux.HandleFunc("DELETE /api/v1/environments/{name}", s.handleDeleteEnvironment)
	s.mux.HandleFunc("POST /api/v1/environments/{name}/heartbeat", s.handleEnvironmentHeartbeat)
	s.mux.HandleFunc("GET /api/v1/environments/{name}/health", s.handleEnvironmentHealth)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS headers for UI consumption.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	log.Printf("%s %s", r.Method, r.URL.Path)
	s.mux.ServeHTTP(w, r)
}

// Start runs the server on the given address.
func (s *Server) Start(addr string) error {
	s.startHealthChecker()
	log.Printf("DCM API server listening on %s", addr)
	return http.ListenAndServe(addr, s)
}

// startHealthChecker runs a background goroutine that actively probes
// each environment's health check endpoint and updates its status.
func (s *Server) startHealthChecker() {
	const defaultInterval = 30 * time.Second
	const defaultTimeout = 10 * time.Second

	go func() {
		ticker := time.NewTicker(defaultInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.probeAllEnvironments(defaultTimeout)
		}
	}()
}

// probeAllEnvironments checks every active environment.
// For kubernetes providers with a kubeconfig, it uses the k8s API directly.
// For others, it uses the explicit health check URL if configured.
func (s *Server) probeAllEnvironments(defaultTimeout time.Duration) {
	envs, err := s.store.ListEnvironments()
	if err != nil {
		log.Printf("[health] error listing environments: %v", err)
		return
	}

	for _, env := range envs {
		if env.Status != "active" {
			continue
		}

		var status, message string

		// Kubernetes providers: use kubeconfig from config to check cluster health.
		if env.Provider == "kubernetes" || env.Provider == "postgres" || env.Provider == "kubevirt" {
			kubeconfig, _ := env.Config["kubeconfig"].(string)
			if kubeconfig == "" {
				continue
			}
			status, message = s.probeKubernetes(kubeconfig, defaultTimeout)
		} else if env.HealthCheck != nil {
			// Other providers: use explicit health check URL.
			var hc types.HealthCheck
			if err := json.Unmarshal(env.HealthCheck, &hc); err != nil {
				log.Printf("[health] %s: invalid health check config: %v", env.Name, err)
				continue
			}
			if hc.URL == "" {
				continue
			}
			status, message = s.probeHTTP(hc, defaultTimeout)
		} else {
			continue
		}

		previousStatus := env.HealthStatus

		if err := s.store.UpdateHealthStatus(env.Name, status, message); err != nil {
			log.Printf("[health] %s: error updating status: %v", env.Name, err)
			continue
		}
		s.registry.UpdateHealthStatus(env.Name, status)

		if status != "healthy" {
			log.Printf("[health] %s: %s — %s", env.Name, status, message)
		}

		// Trigger rehydration when an environment transitions to unhealthy.
		if status == "unhealthy" && previousStatus != "unhealthy" {
			log.Printf("[health] %s: environment became unhealthy, triggering rehydration", env.Name)
			go s.rehydrateFromEnvironment(env.Name)
		}
	}
}

// probeKubernetes checks the health of a Kubernetes cluster using the kubeconfig.
func (s *Server) probeKubernetes(kubeconfig string, defaultTimeout time.Duration) (status, message string) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfig
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return "unhealthy", fmt.Sprintf("invalid kubeconfig: %v", err)
	}
	restConfig.Timeout = defaultTimeout

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "unhealthy", fmt.Sprintf("cannot create k8s client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	body, err := client.Discovery().RESTClient().Get().AbsPath("/healthz").Do(ctx).Raw()
	if err != nil {
		return "unhealthy", fmt.Sprintf("cluster health check failed: %v", err)
	}

	if string(body) == "ok" {
		return "healthy", ""
	}
	return "degraded", fmt.Sprintf("cluster responded: %s", string(body))
}

// probeHTTP performs an HTTP GET to the health check URL.
func (s *Server) probeHTTP(hc types.HealthCheck, defaultTimeout time.Duration) (status, message string) {
	timeout := defaultTimeout
	if hc.TimeoutSeconds > 0 {
		timeout = time.Duration(hc.TimeoutSeconds) * time.Second
	}

	transport := &http.Transport{}
	if hc.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // user-configured
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	req, err := http.NewRequest(http.MethodGet, hc.URL, nil)
	if err != nil {
		return "unhealthy", fmt.Sprintf("invalid health check URL: %v", err)
	}
	for k, v := range hc.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "unhealthy", fmt.Sprintf("probe failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return "healthy", ""
	}
	if resp.StatusCode >= 500 {
		return "unhealthy", fmt.Sprintf("probe returned HTTP %d", resp.StatusCode)
	}
	return "degraded", fmt.Sprintf("probe returned HTTP %d", resp.StatusCode)
}

// --- Helpers ---

// recordToApplication converts a store record back to the engine's Application type.
func recordToApplication(rec *store.ApplicationRecord) *types.Application {
	var components []types.Component
	json.Unmarshal(rec.Components, &components)

	return &types.Application{
		APIVersion: "dcm.io/v1",
		Kind:       "Application",
		Metadata: types.Metadata{
			Name:   rec.Name,
			Labels: rec.Labels,
		},
		Spec: types.ApplicationSpec{
			Components: components,
		},
	}
}

// loadPolicies loads policy records by name and converts them to types.Policy.
func (s *Server) loadPolicies(names []string) ([]types.Policy, error) {
	var policies []types.Policy
	for _, name := range names {
		rec, err := s.store.GetPolicy(name)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", name, err)
		}

		var rules []types.PolicyRule
		json.Unmarshal(rec.Rules, &rules)

		policies = append(policies, types.Policy{
			APIVersion: "dcm.io/v1",
			Kind:       "Policy",
			Metadata:   types.Metadata{Name: rec.Name},
			Spec:       types.PolicySpec{Rules: rules},
		})
	}
	return policies, nil
}

// loadAllPolicies loads every policy from the store.
func (s *Server) loadAllPolicies() ([]types.Policy, error) {
	recs, err := s.store.ListPolicies()
	if err != nil {
		return nil, err
	}
	var policies []types.Policy
	for _, rec := range recs {
		var rules []types.PolicyRule
		json.Unmarshal(rec.Rules, &rules)
		policies = append(policies, types.Policy{
			APIVersion: "dcm.io/v1",
			Kind:       "Policy",
			Metadata:   types.Metadata{Name: rec.Name},
			Spec:       types.PolicySpec{Rules: rules},
		})
	}
	return policies, nil
}
