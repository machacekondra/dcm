package types

import "time"

// State represents the persisted state of all managed resources.
type State struct {
	Version   int                    `json:"version"`
	App       string                 `json:"app"`
	Resources map[string]*Resource   `json:"resources"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// NewState creates an empty state for the given application.
func NewState(appName string) *State {
	return &State{
		Version:   1,
		App:       appName,
		Resources: make(map[string]*Resource),
		UpdatedAt: time.Now(),
	}
}
