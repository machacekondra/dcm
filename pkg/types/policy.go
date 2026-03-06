package types

// Policy defines rules for provider selection and configuration.
type Policy struct {
	APIVersion string     `yaml:"apiVersion" json:"apiVersion"`
	Kind       string     `yaml:"kind" json:"kind"`
	Metadata   Metadata   `yaml:"metadata" json:"metadata"`
	Spec       PolicySpec `yaml:"spec" json:"spec"`
}

type PolicySpec struct {
	Rules []PolicyRule `yaml:"rules" json:"rules"`
}

type PolicyRule struct {
	// Name is an optional human-readable identifier for this rule.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Priority controls rule evaluation order. Higher priority rules are
	// evaluated first and take precedence. Default is 0.
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	Match     PolicyMatch    `yaml:"match" json:"match"`
	Providers ProviderPolicy `yaml:"providers" json:"providers"`

	// Properties are merged into the component properties when this rule matches.
	Properties map[string]any `yaml:"properties,omitempty" json:"properties,omitempty"`
}

type PolicyMatch struct {
	// Type matches against the component type (e.g. "postgres", "container").
	Type string `yaml:"type,omitempty" json:"type,omitempty"`

	// Labels matches against component/application labels. All specified
	// labels must be present for a match.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Expression is a CEL expression for advanced matching. When set,
	// it must evaluate to true for the rule to match. Available variables:
	//   component.name, component.type, component.labels, component.properties
	//   app.name, app.labels
	Expression string `yaml:"expression,omitempty" json:"expression,omitempty"`
}

type ProviderPolicy struct {
	// Preferred lists providers in priority order. The first available
	// provider that supports the resource type will be selected.
	Preferred []string `yaml:"preferred,omitempty" json:"preferred,omitempty"`

	// Required specifies a provider that must be used. If set, overrides
	// preferred and causes an error if the provider is unavailable.
	Required string `yaml:"required,omitempty" json:"required,omitempty"`

	// Forbidden lists providers that must not be used.
	Forbidden []string `yaml:"forbidden,omitempty" json:"forbidden,omitempty"`

	// Strategy selects the provider selection strategy when multiple
	// providers are available: "first", "cheapest", "random".
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}
