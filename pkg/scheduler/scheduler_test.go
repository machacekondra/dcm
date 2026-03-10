package scheduler

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/policy"
	"github.com/dcm-io/dcm/pkg/provider"
	"github.com/dcm-io/dcm/pkg/types"
)

// ---------------------------------------------------------------------------
// Fake provider for testing (avoids importing pkg/provider/mock).
// ---------------------------------------------------------------------------

type fakeProvider struct {
	name         string
	capabilities []types.ResourceType
}

func (f *fakeProvider) Name() string                    { return f.name }
func (f *fakeProvider) Capabilities() []types.ResourceType { return f.capabilities }
func (f *fakeProvider) Plan(_, _ *types.Resource) (*types.Diff, error) {
	return nil, nil
}
func (f *fakeProvider) Apply(_ *types.Diff) (*types.Resource, error) { return nil, nil }
func (f *fakeProvider) Destroy(_ *types.Resource) error              { return nil }
func (f *fakeProvider) Status(_ *types.Resource) (types.ResourceStatus, error) {
	return types.ResourceStatusReady, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testEnv bundles a types.Environment with its provider capabilities so the
// helper can register both the factory and the environment in one shot.
type testEnv struct {
	env          types.Environment
	capabilities []types.ResourceType
}

// testRegistry creates a Registry pre-populated with the supplied environments.
// Each environment's provider type must have a matching factory; the helper
// auto-registers one factory per unique provider type encountered.
func testRegistry(envs ...testEnv) *Registry {
	factories := provider.NewFactoryRegistry()

	// Collect capabilities per provider type so the factory can return the
	// correct fake provider.
	capsByProvider := make(map[string][]types.ResourceType)
	for _, e := range envs {
		pt := e.env.Spec.Provider
		if _, ok := capsByProvider[pt]; !ok {
			capsByProvider[pt] = e.capabilities
		}
	}

	for pt, caps := range capsByProvider {
		// Capture loop vars for closure.
		providerName := pt
		providerCaps := caps
		factories.Register(providerName, func(_ map[string]any) (types.Provider, error) {
			return &fakeProvider{name: providerName, capabilities: providerCaps}, nil
		})
	}

	reg := NewRegistry(factories)
	for _, e := range envs {
		_ = reg.RegisterEnvironment(e.env)
	}
	return reg
}

// makeEnv is a convenience builder for a types.Environment with commonly used
// fields.
func makeEnv(name, providerType string, labels map[string]string, resources *types.ResourcePool, cost *types.CostInfo) types.Environment {
	return types.Environment{
		Metadata: types.Metadata{
			Name:   name,
			Labels: labels,
		},
		Spec: types.EnvironmentSpec{
			Provider:  providerType,
			Resources: resources,
			Cost:      cost,
		},
	}
}

// makeComponent creates a minimal Component for scheduling tests.
func makeComponent(name, compType string, labels map[string]string) *types.Component {
	return &types.Component{
		Name:   name,
		Type:   compType,
		Labels: labels,
	}
}

// makeApp creates a minimal Application for scheduling tests.
func makeApp(name string, labels map[string]string) *types.Application {
	return &types.Application{
		Metadata: types.Metadata{
			Name:   name,
			Labels: labels,
		},
	}
}

// mustScheduler creates a Scheduler and fails the test if construction errors.
func mustScheduler(t *testing.T, reg *Registry, eval *policy.Evaluator) *Scheduler {
	t.Helper()
	s, err := NewScheduler(reg, eval)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	return s
}

// mustEvaluator creates a policy.Evaluator and fails the test on error.
func mustEvaluator(t *testing.T, policies []types.Policy) *policy.Evaluator {
	t.Helper()
	e, err := policy.NewEvaluator(policies)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return e
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSchedule_NoPolicy(t *testing.T) {
	// Without a policy evaluator the scheduler should use strategy "first" and
	// return the first environment that supports the requested resource type.
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-beta", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "redis"},
		},
	)

	s := mustScheduler(t, reg, nil)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	// Strategy should be "first" (the default).
	if result.Strategy != "first" {
		t.Errorf("expected strategy 'first', got %q", result.Strategy)
	}

	// The result must be one of the two environments that support "container".
	if result.Environment != "env-alpha" && result.Environment != "env-beta" {
		t.Errorf("expected environment env-alpha or env-beta, got %q", result.Environment)
	}

	// No matched rules since there is no policy.
	if len(result.MatchedRules) != 0 {
		t.Errorf("expected 0 matched rules, got %d", len(result.MatchedRules))
	}
}

func TestSchedule_NoPolicy_UnsupportedType(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	s := mustScheduler(t, reg, nil)
	comp := makeComponent("cache", "redis", nil)
	app := makeApp("myapp", nil)

	_, err := s.Schedule(comp, app)
	if err == nil {
		t.Fatal("expected error for unsupported resource type, got nil")
	}
}

func TestSchedule_ProviderForbidden(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-kube", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-mock", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres", "redis"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "forbid-kubernetes"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "no-kube",
						Match: types.PolicyMatch{Type: "container"},
						Providers: types.ProviderPolicy{
							Forbidden: []string{"kubernetes"},
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "env-mock" {
		t.Errorf("expected env-mock (kubernetes forbidden), got %q", result.Environment)
	}
	if result.ProviderType != "mock" {
		t.Errorf("expected provider type 'mock', got %q", result.ProviderType)
	}
}

func TestSchedule_ProviderForbidden_AllFiltered(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-kube", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "forbid-all"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "no-kube",
						Match: types.PolicyMatch{Type: "container"},
						Providers: types.ProviderPolicy{
							Forbidden: []string{"kubernetes"},
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	_, err := s.Schedule(comp, app)
	if err == nil {
		t.Fatal("expected error when all providers are forbidden, got nil")
	}
}

func TestSchedule_EnvironmentRequired(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-beta", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "require-beta"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "must-use-beta",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Required: "env-beta",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "env-beta" {
		t.Errorf("expected env-beta (required), got %q", result.Environment)
	}
}

func TestSchedule_EnvironmentRequired_NotFound(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "require-missing"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "must-use-missing",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Required: "env-nonexistent",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	_, err := s.Schedule(comp, app)
	if err == nil {
		t.Fatal("expected error for required-but-missing environment, got nil")
	}
}

func TestSchedule_EnvironmentForbidden(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-beta", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-gamma", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "forbid-envs"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "no-alpha-gamma",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Forbidden: []string{"env-alpha", "env-gamma"},
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "env-beta" {
		t.Errorf("expected env-beta (only non-forbidden), got %q", result.Environment)
	}
}

func TestSchedule_EnvironmentForbidden_AllFiltered(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-alpha", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "forbid-all-envs"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "no-alpha",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Forbidden: []string{"env-alpha"},
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	_, err := s.Schedule(comp, app)
	if err == nil {
		t.Fatal("expected error when all environments are forbidden, got nil")
	}
}

func TestSchedule_CheapestStrategy(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env: makeEnv("expensive", "kubernetes", nil, nil, &types.CostInfo{
				Tier: "premium", HourlyRate: 10.0,
			}),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("mid-range", "mock", nil, nil, &types.CostInfo{
				Tier: "standard", HourlyRate: 5.0,
			}),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("budget", "mock", nil, nil, &types.CostInfo{
				Tier: "spot", HourlyRate: 1.5,
			}),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "cheapest-policy"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "use-cheapest",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy: "cheapest",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Strategy != "cheapest" {
		t.Errorf("expected strategy 'cheapest', got %q", result.Strategy)
	}
	if result.Environment != "budget" {
		t.Errorf("expected 'budget' environment (lowest hourlyRate), got %q", result.Environment)
	}
}

func TestSchedule_CheapestStrategy_NoCostFallback(t *testing.T) {
	// When no cost info is set, the environment is treated as MaxFloat64.
	// An environment with any cost should be preferred.
	reg := testRegistry(
		testEnv{
			env:          makeEnv("no-cost", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("has-cost", "mock", nil, nil, &types.CostInfo{
				Tier: "standard", HourlyRate: 99.0,
			}),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "cheapest"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "cheapest",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy: "cheapest",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "has-cost" {
		t.Errorf("expected 'has-cost' (99.0 < MaxFloat64), got %q", result.Environment)
	}
}

func TestSchedule_LeastLoadedStrategy(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env: makeEnv("small", "kubernetes", nil, &types.ResourcePool{
				CPU: 1000, Memory: 2048, Pods: 10,
			}, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("medium", "mock", nil, &types.ResourcePool{
				CPU: 4000, Memory: 8192, Pods: 50,
			}, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("large", "mock", nil, &types.ResourcePool{
				CPU: 16000, Memory: 32768, Pods: 200,
			}, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "least-loaded-policy"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "use-least-loaded",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy: "least-loaded",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Strategy != "least-loaded" {
		t.Errorf("expected strategy 'least-loaded', got %q", result.Strategy)
	}
	if result.Environment != "large" {
		t.Errorf("expected 'large' environment (most resources), got %q", result.Environment)
	}
}

func TestSchedule_LeastLoadedStrategy_NoResources(t *testing.T) {
	// When no resources are set, capacity is 0. An environment with resources
	// should be preferred.
	reg := testRegistry(
		testEnv{
			env:          makeEnv("no-resources", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("has-resources", "mock", nil, &types.ResourcePool{
				CPU: 100, Memory: 100, Pods: 1,
			}, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "ll"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "ll",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy: "least-loaded",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "has-resources" {
		t.Errorf("expected 'has-resources', got %q", result.Environment)
	}
}

func TestSchedule_MatchExpression(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env: makeEnv("prod-us", "kubernetes",
				map[string]string{"region": "us-east-1", "tier": "production"},
				nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("staging-us", "kubernetes",
				map[string]string{"region": "us-east-1", "tier": "staging"},
				nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("prod-eu", "mock",
				map[string]string{"region": "eu-west-1", "tier": "production"},
				nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	t.Run("filter by label tier==production", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "prod-only"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "prod-envs",
							Match: types.PolicyMatch{Type: "container"},
							Environments: types.EnvironmentPolicy{
								MatchExpression: `environment.labels.tier == "production"`,
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		if result.Environment != "prod-us" && result.Environment != "prod-eu" {
			t.Errorf("expected a production environment, got %q", result.Environment)
		}
	})

	t.Run("filter by region and tier", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "eu-prod"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "eu-prod",
							Match: types.PolicyMatch{Type: "container"},
							Environments: types.EnvironmentPolicy{
								MatchExpression: `environment.labels.region == "eu-west-1" && environment.labels.tier == "production"`,
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		if result.Environment != "prod-eu" {
			t.Errorf("expected prod-eu, got %q", result.Environment)
		}
	})

	t.Run("no match returns error", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "impossible"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "impossible",
							Match: types.PolicyMatch{Type: "container"},
							Environments: types.EnvironmentPolicy{
								MatchExpression: `environment.labels.tier == "nonexistent"`,
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		_, err := s.Schedule(comp, app)
		if err == nil {
			t.Fatal("expected error when no environments match expression, got nil")
		}
	})
}

func TestSchedule_PreferredOrdering(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-a", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env:          makeEnv("env-b", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env:          makeEnv("env-c", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	t.Run("environment preferred list reorders candidates", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "prefer-c"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "prefer-env-c-then-b",
							Match: types.PolicyMatch{Type: "container"},
							Environments: types.EnvironmentPolicy{
								Preferred: []string{"env-c", "env-b"},
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// With strategy "first" (default), the first candidate after ordering
		// by preferred should be env-c.
		if result.Environment != "env-c" {
			t.Errorf("expected env-c (first preferred), got %q", result.Environment)
		}
	})

	t.Run("provider preferred list reorders candidates", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "prefer-mock"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "prefer-mock-provider",
							Match: types.PolicyMatch{Type: "container"},
							Providers: types.ProviderPolicy{
								Preferred: []string{"mock"},
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// mock-backed environments (env-b, env-c) should sort before env-a.
		if result.ProviderType != "mock" {
			t.Errorf("expected provider type 'mock' (preferred), got %q", result.ProviderType)
		}
	})

	t.Run("env preferred takes precedence over provider preferred", func(t *testing.T) {
		policies := []types.Policy{
			{
				Metadata: types.Metadata{Name: "mixed-prefs"},
				Spec: types.PolicySpec{
					Rules: []types.PolicyRule{
						{
							Name:  "mixed",
							Match: types.PolicyMatch{Type: "container"},
							Providers: types.ProviderPolicy{
								Preferred: []string{"mock"},
							},
							Environments: types.EnvironmentPolicy{
								Preferred: []string{"env-a"},
							},
						},
					},
				},
			},
		}

		eval := mustEvaluator(t, policies)
		s := mustScheduler(t, reg, eval)
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// env-a should win because environment preferred scores lower (better)
		// than provider preferred.
		if result.Environment != "env-a" {
			t.Errorf("expected env-a (env preferred overrides provider preferred), got %q", result.Environment)
		}
	})
}

func TestSchedule_ProviderRequired(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-kube", "kubernetes", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
		testEnv{
			env:          makeEnv("env-mock", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "require-mock"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "must-mock",
						Match: types.PolicyMatch{Type: "postgres"},
						Providers: types.ProviderPolicy{
							Required: "mock",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("db", "postgres", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.ProviderType != "mock" {
		t.Errorf("expected provider type 'mock' (required), got %q", result.ProviderType)
	}
	if result.Environment != "env-mock" {
		t.Errorf("expected env-mock, got %q", result.Environment)
	}
}

func TestSchedule_MatchedRulesReported(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-default", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "multi-rule"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "rule-a",
						Match: types.PolicyMatch{Type: "container"},
					},
					{
						Name:  "rule-b",
						Match: types.PolicyMatch{}, // matches everything
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if len(result.MatchedRules) != 2 {
		t.Fatalf("expected 2 matched rules, got %d: %v", len(result.MatchedRules), result.MatchedRules)
	}
	if result.MatchedRules[0] != "rule-a" {
		t.Errorf("expected first matched rule 'rule-a', got %q", result.MatchedRules[0])
	}
	if result.MatchedRules[1] != "rule-b" {
		t.Errorf("expected second matched rule 'rule-b', got %q", result.MatchedRules[1])
	}
}

func TestSchedule_PropertiesMerged(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-default", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "props-policy"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:       "set-replicas",
						Match:      types.PolicyMatch{Type: "container"},
						Properties: map[string]any{"replicas": 3},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Properties == nil {
		t.Fatal("expected non-nil Properties")
	}
	if v, ok := result.Properties["replicas"]; !ok || v != 3 {
		t.Errorf("expected properties[replicas]=3, got %v", v)
	}
}

func TestSchedule_RegisterProvider_BackwardCompat(t *testing.T) {
	// Use RegisterProvider (no factory needed) for backward-compat mode.
	factories := provider.NewFactoryRegistry()
	reg := NewRegistry(factories)
	fp := &fakeProvider{
		name:         "docker",
		capabilities: []types.ResourceType{"container"},
	}
	reg.RegisterProvider(fp)

	s := mustScheduler(t, reg, nil)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if result.Environment != "docker" {
		t.Errorf("expected environment 'docker' (from RegisterProvider), got %q", result.Environment)
	}
	if result.Provider != fp {
		t.Error("expected the same Provider instance returned")
	}
}

func TestSchedule_CombinedFilters(t *testing.T) {
	// Test combining provider forbidden + environment forbidden + match expression.
	reg := testRegistry(
		testEnv{
			env: makeEnv("prod-kube", "kubernetes",
				map[string]string{"tier": "production"}, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("staging-kube", "kubernetes",
				map[string]string{"tier": "staging"}, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("prod-mock", "mock",
				map[string]string{"tier": "production"}, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env: makeEnv("staging-mock", "mock",
				map[string]string{"tier": "staging"}, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "combined"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "production-mock-only",
						Match: types.PolicyMatch{Type: "container"},
						Providers: types.ProviderPolicy{
							Forbidden: []string{"kubernetes"},
						},
						Environments: types.EnvironmentPolicy{
							MatchExpression: `environment.labels.tier == "production"`,
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	result, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	// Only prod-mock should survive: kubernetes forbidden, staging filtered by expression.
	if result.Environment != "prod-mock" {
		t.Errorf("expected prod-mock, got %q", result.Environment)
	}
}

func TestSchedule_LabelMatch(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-default", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container", "postgres"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "label-match"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name: "only-critical",
						Match: types.PolicyMatch{
							Labels: map[string]string{"criticality": "high"},
						},
						Environments: types.EnvironmentPolicy{
							Strategy: "cheapest",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)

	t.Run("matching labels", func(t *testing.T) {
		comp := makeComponent("db", "postgres", map[string]string{"criticality": "high"})
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		// Rule matched, so strategy should be cheapest.
		if result.Strategy != "cheapest" {
			t.Errorf("expected strategy 'cheapest', got %q", result.Strategy)
		}
	})

	t.Run("non-matching labels", func(t *testing.T) {
		comp := makeComponent("db", "postgres", map[string]string{"criticality": "low"})
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		// Rule did not match, so strategy should be default "first".
		if result.Strategy != "first" {
			t.Errorf("expected strategy 'first' (rule not matched), got %q", result.Strategy)
		}
	})
}

func TestSchedule_RoundRobinStrategy(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-a", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
		testEnv{
			env:          makeEnv("env-b", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "rr"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "round-robin",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy:  "round-robin",
							Preferred: []string{"env-a", "env-b"},
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	// Schedule twice; with round-robin the index increments each call.
	r1, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule 1: %v", err)
	}
	r2, err := s.Schedule(comp, app)
	if err != nil {
		t.Fatalf("Schedule 2: %v", err)
	}

	// The two results should select different environments (assuming preferred
	// ordering gives us deterministic candidate order).
	if r1.Environment == r2.Environment {
		t.Errorf("round-robin should rotate environments, but both returned %q", r1.Environment)
	}
}

// makeEnvWithCaps creates an environment with capabilities.
func makeEnvWithCaps(name, providerType string, caps []string) types.Environment {
	return types.Environment{
		Metadata: types.Metadata{Name: name},
		Spec: types.EnvironmentSpec{
			Provider:     providerType,
			Capabilities: caps,
		},
	}
}

func TestSchedule_RequirementsFilter(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnvWithCaps("prod-k8s", "kubernetes", []string{"loadbalancer", "persistent-storage"}),
			capabilities: []types.ResourceType{"container", "ip"},
		},
		testEnv{
			env:          makeEnvWithCaps("dev-k8s", "kubernetes", []string{"persistent-storage"}),
			capabilities: []types.ResourceType{"container", "ip"},
		},
	)

	s := mustScheduler(t, reg, nil)

	t.Run("component requiring loadbalancer gets prod-k8s", func(t *testing.T) {
		comp := &types.Component{
			Name:     "app",
			Type:     "container",
			Requires: []string{"loadbalancer"},
		}
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		if result.Environment != "prod-k8s" {
			t.Errorf("expected prod-k8s (only env with loadbalancer), got %q", result.Environment)
		}
	})

	t.Run("component requiring both capabilities gets prod-k8s", func(t *testing.T) {
		comp := &types.Component{
			Name:     "app",
			Type:     "container",
			Requires: []string{"loadbalancer", "persistent-storage"},
		}
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		if result.Environment != "prod-k8s" {
			t.Errorf("expected prod-k8s, got %q", result.Environment)
		}
	})

	t.Run("component requiring unavailable capability fails", func(t *testing.T) {
		comp := &types.Component{
			Name:     "ml",
			Type:     "container",
			Requires: []string{"gpu"},
		}
		app := makeApp("myapp", nil)

		_, err := s.Schedule(comp, app)
		if err == nil {
			t.Fatal("expected error for unsatisfiable requirement")
		}
	})

	t.Run("component with no requirements gets any env", func(t *testing.T) {
		comp := makeComponent("web", "container", nil)
		app := makeApp("myapp", nil)

		result, err := s.Schedule(comp, app)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		if result.Environment != "prod-k8s" && result.Environment != "dev-k8s" {
			t.Errorf("expected any env, got %q", result.Environment)
		}
	})
}

func TestSchedule_UnknownStrategy(t *testing.T) {
	reg := testRegistry(
		testEnv{
			env:          makeEnv("env-default", "mock", nil, nil, nil),
			capabilities: []types.ResourceType{"container"},
		},
	)

	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "bad-strategy"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{
					{
						Name:  "bad",
						Match: types.PolicyMatch{Type: "container"},
						Environments: types.EnvironmentPolicy{
							Strategy: "nonexistent-strategy",
						},
					},
				},
			},
		},
	}

	eval := mustEvaluator(t, policies)
	s := mustScheduler(t, reg, eval)
	comp := makeComponent("web", "container", nil)
	app := makeApp("myapp", nil)

	_, err := s.Schedule(comp, app)
	if err == nil {
		t.Fatal("expected error for unknown scheduling strategy, got nil")
	}
}
