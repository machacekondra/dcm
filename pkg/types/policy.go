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
	Match     PolicyMatch     `yaml:"match" json:"match"`
	Providers ProviderPolicy  `yaml:"providers" json:"providers"`
	Properties map[string]interface{} `yaml:"properties,omitempty" json:"properties,omitempty"`
}

type PolicyMatch struct {
	Type   string            `yaml:"type,omitempty" json:"type,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}

type ProviderPolicy struct {
	Preferred []string `yaml:"preferred,omitempty" json:"preferred,omitempty"`
	Forbidden []string `yaml:"forbidden,omitempty" json:"forbidden,omitempty"`
	Strategy  string   `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}
