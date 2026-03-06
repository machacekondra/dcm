package types

// Application represents a deployable application composed of multiple components.
type Application struct {
	APIVersion string            `yaml:"apiVersion" json:"apiVersion"`
	Kind       string            `yaml:"kind" json:"kind"`
	Metadata   Metadata          `yaml:"metadata" json:"metadata"`
	Spec       ApplicationSpec   `yaml:"spec" json:"spec"`
}

type ApplicationSpec struct {
	Components []Component `yaml:"components" json:"components"`
}

// Component is a single deployable unit within an application.
type Component struct {
	Name       string                 `yaml:"name" json:"name"`
	Type       string                 `yaml:"type" json:"type"`
	DependsOn  []string               `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	Labels     map[string]string      `yaml:"labels,omitempty" json:"labels,omitempty"`
	Properties map[string]interface{} `yaml:"properties,omitempty" json:"properties,omitempty"`
}

type Metadata struct {
	Name      string            `yaml:"name" json:"name"`
	Namespace string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
}
