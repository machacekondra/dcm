package api

import (
	"net/http"
)

// PropertySchema describes a single property for a component type.
type PropertySchema struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "string", "number", "boolean", "object"
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

// TypeSchema describes a component type and its properties.
type TypeSchema struct {
	Type        string           `json:"type"`
	Description string           `json:"description"`
	Providers   []string         `json:"providers"`
	Properties  []PropertySchema `json:"properties"`
}

// builtinTypeSchemas returns the known type schemas.
// This is the single source of truth for component type metadata.
func builtinTypeSchemas(providerNames []string) []TypeSchema {
	// Helper to find which providers support a type.
	// For now we use known mappings; later this could be dynamic.
	mockOnly := filterProviders(providerNames, "mock")
	allProviders := filterProviders(providerNames, "kubernetes", "kubevirt", "mock")

	return []TypeSchema{
		{
			Type:        "container",
			Description: "Container workload (Kubernetes Deployment + Service)",
			Providers:   allProviders,
			Properties: []PropertySchema{
				{Name: "image", Type: "string", Required: true, Description: "Container image (e.g. nginx:latest)"},
				{Name: "replicas", Type: "number", Default: 1, Description: "Number of replicas"},
				{Name: "port", Type: "number", Default: 8080, Description: "Container port to expose"},
				{Name: "env", Type: "object", Description: "Environment variables as key-value pairs"},
			},
		},
		{
			Type:        "postgres",
			Description: "PostgreSQL database instance",
			Providers:   filterProviders(providerNames, "postgres", "mock"),
			Properties: []PropertySchema{
				{Name: "version", Type: "string", Default: "16", Description: "PostgreSQL version"},
				{Name: "maxConnections", Type: "number", Default: 100, Description: "Maximum connections"},
				{Name: "storage", Type: "string", Default: "1Gi", Description: "Storage size (PVC)"},
				{Name: "database", Type: "string", Default: "app", Description: "Database name to create"},
				{Name: "username", Type: "string", Default: "postgres", Description: "Database user"},
				{Name: "password", Type: "string", Default: "postgres", Description: "Database password"},
			},
		},
		{
			Type:        "redis",
			Description: "Redis in-memory cache",
			Providers:   mockOnly,
			Properties: []PropertySchema{
				{Name: "version", Type: "string", Default: "7", Description: "Redis version"},
				{Name: "maxMemory", Type: "string", Default: "256mb", Description: "Maximum memory"},
			},
		},
		{
			Type:        "static-site",
			Description: "Static website hosting",
			Providers:   mockOnly,
			Properties: []PropertySchema{
				{Name: "source", Type: "string", Required: true, Description: "Path or URL to static content"},
				{Name: "domain", Type: "string", Description: "Custom domain name"},
			},
		},
		{
			Type:        "network",
			Description: "Network resource (VPC, subnet, etc.)",
			Providers:   mockOnly,
			Properties: []PropertySchema{
				{Name: "cidr", Type: "string", Default: "10.0.0.0/16", Description: "CIDR block"},
				{Name: "public", Type: "boolean", Default: false, Description: "Whether the network is public"},
			},
		},
		{
			Type:        "storage",
			Description: "Persistent storage volume",
			Providers:   mockOnly,
			Properties: []PropertySchema{
				{Name: "size", Type: "string", Default: "10Gi", Description: "Storage size"},
				{Name: "storageClass", Type: "string", Default: "standard", Description: "Storage class"},
				{Name: "accessMode", Type: "string", Default: "ReadWriteOnce", Description: "Access mode"},
			},
		},
		{
			Type:        "ip",
			Description: "IP address reservation (IPAM)",
			Providers:   filterProviders(providerNames, "static-ipam", "mock"),
			Properties: []PropertySchema{
				{Name: "pool", Type: "string", Required: true, Description: "IPAM pool or subnet name"},
				{Name: "version", Type: "string", Default: "4", Description: "IP version: 4 or 6"},
				{Name: "attachTo", Type: "string", Description: "Resource to bind the IP to (output reference)"},
			},
		},
		{
			Type:        "dns",
			Description: "DNS record management",
			Providers:   filterProviders(providerNames, "powerdns", "mock"),
			Properties: []PropertySchema{
				{Name: "zone", Type: "string", Required: true, Description: "DNS zone (e.g. example.com)"},
				{Name: "record", Type: "string", Required: true, Description: "FQDN (e.g. web.example.com)"},
				{Name: "type", Type: "string", Required: true, Description: "Record type: A, AAAA, CNAME"},
				{Name: "value", Type: "string", Required: true, Description: "Record value (IP or hostname, supports output references)"},
				{Name: "ttl", Type: "number", Default: 300, Description: "TTL in seconds"},
			},
		},
		{
			Type:        "vm",
			Description: "Virtual machine (KubeVirt VirtualMachine)",
			Providers:   filterProviders(providerNames, "kubevirt", "mock"),
			Properties: []PropertySchema{
				{Name: "image", Type: "string", Required: true, Description: "VM disk image (container disk image reference)"},
				{Name: "cpu", Type: "number", Default: 1, Description: "Number of vCPUs"},
				{Name: "memoryMB", Type: "number", Default: 1024, Description: "Memory in MB"},
				{Name: "memory", Type: "string", Description: "Memory with unit (e.g. 2Gi) — overrides memoryMB"},
				{Name: "userData", Type: "string", Description: "Cloud-init user data"},
				{Name: "sshKey", Type: "string", Description: "SSH public key for access"},
				{Name: "network", Type: "string", Description: "Network name (Multus). Default: pod network"},
			},
		},
	}
}

func filterProviders(available []string, names ...string) []string {
	var result []string
	for _, n := range names {
		for _, a := range available {
			if a == n {
				result = append(result, n)
				break
			}
		}
	}
	return result
}

func (s *Server) handleListTypes(w http.ResponseWriter, r *http.Request) {
	providers := s.registry.ListProviders()
	var names []string
	for _, p := range providers {
		names = append(names, p.Name())
	}
	schemas := builtinTypeSchemas(names)
	writeJSON(w, http.StatusOK, schemas)
}
