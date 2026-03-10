package cmd

import (
	"encoding/json"
	"log"

	"github.com/dcm-io/dcm/pkg/store"
	"github.com/dcm-io/dcm/pkg/types"
)

// seedSampleData creates sample applications, environments, and policies
// if they don't already exist. This provides a useful starting point
// for new users exploring the UI.
func seedSampleData(db *store.Store) {
	seedSampleEnvironments(db)
	seedPetclinicApp(db)
}

func seedPetclinicApp(db *store.Store) {
	const name = "spring-petclinic"

	if _, err := db.GetApplication(name); err == nil {
		return // already exists
	}

	components := []types.Component{
		{
			Name:     "database",
			Type:     "postgres",
			Requires: []string{"persistent-storage"},
			Labels: map[string]string{
				"tier": "data",
			},
			ColocateWith: "app",
			Properties: map[string]any{
				"version":        "16",
				"storage":        "20Gi",
				"maxConnections": 200,
			},
		},
		{
			Name:      "app",
			Type:      "container",
			DependsOn: []string{"database", "cache"},
			Requires:  []string{"loadbalancer"},
			Labels: map[string]string{
				"tier": "backend",
			},
			Properties: map[string]any{
				"image":    "springcommunity/spring-petclinic:latest",
				"replicas": 2,
				"port":     8080,
				"env": map[string]any{
					"SPRING_PROFILES_ACTIVE":                    "postgres",
					"SPRING_DATASOURCE_URL":                     "${database.outputs.connectionString}",
					"SPRING_DATASOURCE_USERNAME":                "petclinic",
					"SPRING_DATASOURCE_PASSWORD":                "petclinic",
					"SPRING_CACHE_TYPE":                         "redis",
					"SPRING_DATA_REDIS_HOST":                    "${cache.outputs.host}",
					"SPRING_DATA_REDIS_PORT":                    "${cache.outputs.port}",
					"MANAGEMENT_ENDPOINTS_WEB_EXPOSURE_INCLUDE": "health,info,prometheus",
				},
			},
		},
		{
			Name:         "app-ip",
			Type:         "ip",
			DependsOn:    []string{"app"},
			Requires:     []string{"loadbalancer"},
			ColocateWith: "app",
			Labels: map[string]string{
				"tier": "network",
			},
			Properties: map[string]any{
				"pool": "production",
			},
		},
		{
			Name:      "app-dns",
			Type:      "dns",
			DependsOn: []string{"app-ip"},
			Labels: map[string]string{
				"tier": "network",
			},
			Properties: map[string]any{
				"zone":   "example.com",
				"record": "petclinic.example.com",
				"type":   "A",
				"value":  "${app-ip.outputs.address}",
				"ttl":    300,
			},
		},
	}

	componentsJSON, _ := json.Marshal(components)
	rec := &store.ApplicationRecord{
		Name: name,
		Labels: map[string]string{
			"framework": "spring-boot",
			"team":      "platform",
		},
		Components: componentsJSON,
	}

	if err := db.CreateApplication(rec); err != nil {
		log.Printf("seed: failed to create %s: %v", name, err)
		return
	}
	log.Printf("seed: created sample application %q (3-tier web app with IP/DNS)", name)
}

func seedSampleEnvironments(db *store.Store) {
	envs := []store.EnvironmentRecord{
		{
			Name:     "dev-cluster",
			Provider: "mock",
			Labels: map[string]string{
				"env":    "development",
				"region": "us-east-1",
			},
			Capabilities: []string{"loadbalancer", "persistent-storage"},
			Config:       map[string]any{},
			Status:       "active",
		},
	}

	for _, env := range envs {
		if _, err := db.GetEnvironment(env.Name); err == nil {
			continue
		}
		if err := db.CreateEnvironment(&env); err != nil {
			log.Printf("seed: failed to create environment %s: %v", env.Name, err)
			continue
		}
		log.Printf("seed: created sample environment %q (capabilities: %v)", env.Name, env.Capabilities)
	}
}
