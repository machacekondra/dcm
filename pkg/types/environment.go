package types

// Environment represents a configured instance of a provider type.
type Environment struct {
	APIVersion string          `yaml:"apiVersion" json:"apiVersion"`
	Kind       string          `yaml:"kind" json:"kind"`
	Metadata   Metadata        `yaml:"metadata" json:"metadata"`
	Spec       EnvironmentSpec `yaml:"spec" json:"spec"`
}

type EnvironmentSpec struct {
	// Provider is the provider type (e.g., "kubernetes", "mock", "aws").
	Provider string `yaml:"provider" json:"provider"`

	// Capabilities lists infrastructure features this environment provides
	// (e.g., "loadbalancer", "persistent-storage", "gpu").
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`

	// Config holds provider-specific configuration.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`

	// Resources describes available capacity for scheduling.
	Resources *ResourcePool `yaml:"resources,omitempty" json:"resources,omitempty"`

	// Cost describes cost metadata for scheduling.
	Cost *CostInfo `yaml:"cost,omitempty" json:"cost,omitempty"`
}

// ResourcePool describes available capacity in an environment.
type ResourcePool struct {
	CPU    int `yaml:"cpu" json:"cpu"`       // millicores
	Memory int `yaml:"memory" json:"memory"` // MB
	Pods   int `yaml:"pods" json:"pods"`
}

// CostInfo describes cost metadata for an environment.
type CostInfo struct {
	Tier       string  `yaml:"tier" json:"tier"`             // standard, premium, spot
	HourlyRate float64 `yaml:"hourlyRate" json:"hourlyRate"` // cost per unit per hour
}
