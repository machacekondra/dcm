package types

// ResourceType identifies a type of resource a provider can manage.
type ResourceType string

const (
	ResourceTypeContainer  ResourceType = "container"
	ResourceTypePostgres   ResourceType = "postgres"
	ResourceTypeRedis      ResourceType = "redis"
	ResourceTypeStaticSite ResourceType = "static-site"
	ResourceTypeNetwork    ResourceType = "network"
	ResourceTypeStorage    ResourceType = "storage"
)

// Resource represents a single managed resource and its current state.
type Resource struct {
	Name        string                 `json:"name"`
	Type        ResourceType           `json:"type"`
	Provider    string                 `json:"provider"`
	Environment string                 `json:"environment,omitempty"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
	Status      ResourceStatus         `json:"status"`
}

type ResourceStatus string

const (
	ResourceStatusPending  ResourceStatus = "pending"
	ResourceStatusCreating ResourceStatus = "creating"
	ResourceStatusReady    ResourceStatus = "ready"
	ResourceStatusUpdating ResourceStatus = "updating"
	ResourceStatusDeleting ResourceStatus = "deleting"
	ResourceStatusFailed   ResourceStatus = "failed"
	ResourceStatusUnknown  ResourceStatus = "unknown"
)

// DiffAction represents the type of change to apply.
type DiffAction string

const (
	DiffActionCreate  DiffAction = "create"
	DiffActionUpdate  DiffAction = "update"
	DiffActionDelete  DiffAction = "delete"
	DiffActionNone    DiffAction = "none"
)

// Diff describes the difference between desired and current state.
type Diff struct {
	Action      DiffAction             `json:"action"`
	Resource    string                 `json:"resource"`
	Type        ResourceType           `json:"type"`
	Provider    string                 `json:"provider"`
	Environment string                 `json:"environment,omitempty"`
	Before      map[string]interface{} `json:"before,omitempty"`
	After       map[string]interface{} `json:"after,omitempty"`
}

// Provider is the interface that all infrastructure providers must implement.
type Provider interface {
	// Name returns the provider's unique identifier.
	Name() string

	// Capabilities returns the resource types this provider can manage.
	Capabilities() []ResourceType

	// Plan computes the diff between desired and current state.
	Plan(desired, current *Resource) (*Diff, error)

	// Apply executes a planned diff and returns the resulting resource.
	Apply(diff *Diff) (*Resource, error)

	// Destroy removes a resource.
	Destroy(resource *Resource) error

	// Status checks the current status of a resource.
	Status(resource *Resource) (ResourceStatus, error)
}
