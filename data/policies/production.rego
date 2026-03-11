package dcm.compliance

# Production databases must have at least 10Gi storage.
deny contains msg if {
	input.component.type == "postgres"
	input.environment.labels.env == "prod"
	storage := input.component.properties.storage
	not startswith(storage, "10")
	not startswith(storage, "20")
	not startswith(storage, "50")
	not startswith(storage, "100")
	msg := sprintf("postgres %q in prod requires at least 10Gi storage, got %s", [input.component.name, storage])
}

# Production containers must specify replicas.
deny contains msg if {
	input.component.type == "container"
	not input.component.properties.replicas
	input.environment.labels.env == "prod"
	msg := sprintf("container %q in prod must specify replicas", [input.component.name])
}

# Environments must not exceed cost limit.
deny contains msg if {
	input.environment.cost.hourlyRate > 1.0
	msg := sprintf("environment %q exceeds cost limit ($%.2f/hr)", [input.environment.name, input.environment.cost.hourlyRate])
}
