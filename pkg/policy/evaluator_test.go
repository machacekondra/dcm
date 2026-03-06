package policy

import (
	"testing"

	"github.com/dcm-io/dcm/pkg/types"
)

func makeApp(name string, labels map[string]string) *types.Application {
	return &types.Application{
		Metadata: types.Metadata{Name: name, Labels: labels},
	}
}

func makeComponent(name, typ string, labels map[string]string) *types.Component {
	return &types.Component{Name: name, Type: typ, Labels: labels}
}

func TestEvaluator_NoRulesMatch(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "test"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Match:     types.PolicyMatch{Type: "postgres"},
				Providers: types.ProviderPolicy{Preferred: []string{"aws"}},
			}},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("web", "container", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.MatchedRules) != 0 {
		t.Errorf("expected no matched rules, got %v", result.MatchedRules)
	}
	if len(result.Preferred) != 0 {
		t.Errorf("expected no preferred, got %v", result.Preferred)
	}
}

func TestEvaluator_TypeMatch(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "db-policy"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Name:      "prefer-aws-for-postgres",
				Match:     types.PolicyMatch{Type: "postgres"},
				Providers: types.ProviderPolicy{Preferred: []string{"aws"}},
			}},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("db", "postgres", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.MatchedRules) != 1 {
		t.Fatalf("expected 1 matched rule, got %v", result.MatchedRules)
	}
	if result.Preferred[0] != "aws" {
		t.Errorf("expected preferred [aws], got %v", result.Preferred)
	}
}

func TestEvaluator_LabelMatch(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "placement"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Match: types.PolicyMatch{
					Labels: map[string]string{"env": "production"},
				},
				Providers: types.ProviderPolicy{
					Forbidden: []string{"mock"},
				},
			}},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	// App-level label should be inherited by components.
	result, err := eval.Evaluate(
		makeComponent("web", "container", nil),
		makeApp("app", map[string]string{"env": "production"}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Forbidden) != 1 || result.Forbidden[0] != "mock" {
		t.Errorf("expected forbidden [mock], got %v", result.Forbidden)
	}
}

func TestEvaluator_CELExpression(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "cel-policy"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Name: "high-replica-containers",
				Match: types.PolicyMatch{
					Expression: `component.type == "container" && component.name == "backend"`,
				},
				Providers: types.ProviderPolicy{Preferred: []string{"kubernetes"}},
			}},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	// Should match.
	result, err := eval.Evaluate(
		makeComponent("backend", "container", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MatchedRules) != 1 {
		t.Errorf("expected 1 match, got %d", len(result.MatchedRules))
	}

	// Should not match (wrong name).
	result, err = eval.Evaluate(
		makeComponent("frontend", "container", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.MatchedRules) != 0 {
		t.Errorf("expected 0 matches, got %d", len(result.MatchedRules))
	}
}

func TestEvaluator_CELWithLabels(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "cel-labels"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Match: types.PolicyMatch{
					Expression: `component.labels.tier == "critical"`,
				},
				Providers: types.ProviderPolicy{Required: "aws"},
			}},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("db", "postgres", map[string]string{"tier": "critical"}),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Required != "aws" {
		t.Errorf("expected required=aws, got %q", result.Required)
	}
}

func TestEvaluator_InvalidCELExpression(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "bad"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{{
				Match: types.PolicyMatch{Expression: "invalid %%% syntax"},
			}},
		},
	}}

	_, err := NewEvaluator(policies)
	if err == nil {
		t.Fatal("expected error for invalid CEL expression")
	}
}

func TestEvaluator_PriorityOrdering(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "priorities"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{
				{
					Name:     "low-priority",
					Priority: 10,
					Match:    types.PolicyMatch{Type: "container"},
					Providers: types.ProviderPolicy{
						Preferred: []string{"mock"},
						Strategy:  "first",
					},
				},
				{
					Name:     "high-priority",
					Priority: 100,
					Match:    types.PolicyMatch{Type: "container"},
					Providers: types.ProviderPolicy{
						Preferred: []string{"kubernetes"},
						Strategy:  "cheapest",
					},
				},
			},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("web", "container", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	// High-priority rule's preferred should come first.
	if len(result.Preferred) != 2 || result.Preferred[0] != "kubernetes" {
		t.Errorf("expected preferred [kubernetes, mock], got %v", result.Preferred)
	}

	// Strategy from highest priority rule wins.
	if result.Strategy != "cheapest" {
		t.Errorf("expected strategy 'cheapest', got %q", result.Strategy)
	}

	// Both rules matched.
	if len(result.MatchedRules) != 2 {
		t.Errorf("expected 2 matched rules, got %d", len(result.MatchedRules))
	}
}

func TestEvaluator_ForbiddenOverridesPreferred(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "conflict"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{
				{
					Match:     types.PolicyMatch{Type: "container"},
					Providers: types.ProviderPolicy{Preferred: []string{"mock", "kubernetes"}},
				},
				{
					Match:     types.PolicyMatch{Labels: map[string]string{"env": "production"}},
					Providers: types.ProviderPolicy{Forbidden: []string{"mock"}},
				},
			},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("web", "container", map[string]string{"env": "production"}),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	// mock should be removed from preferred since it's forbidden.
	if len(result.Preferred) != 1 || result.Preferred[0] != "kubernetes" {
		t.Errorf("expected preferred [kubernetes], got %v", result.Preferred)
	}
	if !result.IsProviderAllowed("kubernetes") {
		t.Error("kubernetes should be allowed")
	}
	if result.IsProviderAllowed("mock") {
		t.Error("mock should not be allowed")
	}
}

func TestEvaluator_RequiredAndForbiddenConflict(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "conflict"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{
				{
					Match:     types.PolicyMatch{Type: "postgres"},
					Providers: types.ProviderPolicy{Required: "aws"},
				},
				{
					Match:     types.PolicyMatch{Type: "postgres"},
					Providers: types.ProviderPolicy{Forbidden: []string{"aws"}},
				},
			},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	_, err = eval.Evaluate(
		makeComponent("db", "postgres", nil),
		makeApp("app", nil),
	)
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestEvaluator_PropertyMerge(t *testing.T) {
	policies := []types.Policy{{
		Metadata: types.Metadata{Name: "props"},
		Spec: types.PolicySpec{
			Rules: []types.PolicyRule{
				{
					Priority:   100,
					Match:      types.PolicyMatch{Type: "postgres"},
					Properties: map[string]any{"multiAz": true, "version": "16"},
				},
				{
					Priority:   50,
					Match:      types.PolicyMatch{Type: "postgres"},
					Properties: map[string]any{"backupRetention": "30d", "version": "15"},
				},
			},
		},
	}}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("db", "postgres", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	// multiAz from high-priority rule.
	if result.Properties["multiAz"] != true {
		t.Error("expected multiAz=true")
	}
	// backupRetention from low-priority (not overridden by high-priority).
	if result.Properties["backupRetention"] != "30d" {
		t.Error("expected backupRetention=30d")
	}
	// version from high-priority should win.
	if result.Properties["version"] != "16" {
		t.Errorf("expected version=16, got %v", result.Properties["version"])
	}
}

func TestEvaluator_Deduplication(t *testing.T) {
	policies := []types.Policy{
		{
			Metadata: types.Metadata{Name: "p1"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{{
					Match:     types.PolicyMatch{Type: "container"},
					Providers: types.ProviderPolicy{Preferred: []string{"aws", "gcp"}},
				}},
			},
		},
		{
			Metadata: types.Metadata{Name: "p2"},
			Spec: types.PolicySpec{
				Rules: []types.PolicyRule{{
					Match:     types.PolicyMatch{Type: "container"},
					Providers: types.ProviderPolicy{Preferred: []string{"gcp", "azure"}},
				}},
			},
		},
	}

	eval, err := NewEvaluator(policies)
	if err != nil {
		t.Fatal(err)
	}

	result, err := eval.Evaluate(
		makeComponent("web", "container", nil),
		makeApp("app", nil),
	)
	if err != nil {
		t.Fatal(err)
	}

	// gcp should appear only once.
	count := 0
	for _, p := range result.Preferred {
		if p == "gcp" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected gcp once in preferred, got %d times in %v", count, result.Preferred)
	}
}

// --- SelectProvider tests ---

type fakeProvider struct {
	name         string
	capabilities []types.ResourceType
}

func (f *fakeProvider) Name() string                  { return f.name }
func (f *fakeProvider) Capabilities() []types.ResourceType { return f.capabilities }
func (f *fakeProvider) Plan(_, _ *types.Resource) (*types.Diff, error) { return nil, nil }
func (f *fakeProvider) Apply(_ *types.Diff) (*types.Resource, error)  { return nil, nil }
func (f *fakeProvider) Destroy(_ *types.Resource) error               { return nil }
func (f *fakeProvider) Status(_ *types.Resource) (types.ResourceStatus, error) {
	return types.ResourceStatusReady, nil
}

type fakeRegistry struct {
	providers map[string]types.Provider
}

func (r *fakeRegistry) Get(name string) (types.Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

func (r *fakeRegistry) ListProviders() []types.Provider {
	var out []types.Provider
	for _, p := range r.providers {
		out = append(out, p)
	}
	return out
}

func newFakeRegistry(providers ...types.Provider) *fakeRegistry {
	r := &fakeRegistry{providers: make(map[string]types.Provider)}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	return r
}

func TestSelectProvider_Required(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "aws", capabilities: []types.ResourceType{"postgres"}},
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"postgres"}},
	)

	result := &Result{Required: "aws"}
	p, err := SelectProvider(result, reg, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "aws" {
		t.Errorf("expected aws, got %s", p.Name())
	}
}

func TestSelectProvider_RequiredNotRegistered(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"postgres"}},
	)

	result := &Result{Required: "aws"}
	_, err := SelectProvider(result, reg, "postgres")
	if err == nil {
		t.Fatal("expected error for unregistered required provider")
	}
}

func TestSelectProvider_PreferredOrder(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "aws", capabilities: []types.ResourceType{"container"}},
		&fakeProvider{name: "kubernetes", capabilities: []types.ResourceType{"container"}},
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"container"}},
	)

	result := &Result{Preferred: []string{"kubernetes", "aws"}}
	p, err := SelectProvider(result, reg, "container")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "kubernetes" {
		t.Errorf("expected kubernetes (first preferred), got %s", p.Name())
	}
}

func TestSelectProvider_ForbiddenFiltering(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"container"}},
		&fakeProvider{name: "kubernetes", capabilities: []types.ResourceType{"container"}},
	)

	result := &Result{Forbidden: []string{"mock"}}
	p, err := SelectProvider(result, reg, "container")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "kubernetes" {
		t.Errorf("expected kubernetes (mock forbidden), got %s", p.Name())
	}
}

func TestSelectProvider_AllForbidden(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"container"}},
	)

	result := &Result{Forbidden: []string{"mock"}}
	_, err := SelectProvider(result, reg, "container")
	if err == nil {
		t.Fatal("expected error when all providers are forbidden")
	}
}

func TestSelectProvider_FallbackNoPolicy(t *testing.T) {
	reg := newFakeRegistry(
		&fakeProvider{name: "mock", capabilities: []types.ResourceType{"container", "postgres"}},
	)

	result := &Result{}
	p, err := SelectProvider(result, reg, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected mock as fallback, got %s", p.Name())
	}
}
