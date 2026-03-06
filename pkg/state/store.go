package state

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dcm-io/dcm/pkg/types"
)

// Store handles persistence of application state.
type Store struct {
	path string
}

// NewStore creates a store that reads/writes state to the given file path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultPath returns the default state file path for an application.
func DefaultPath(appName string) string {
	return fmt.Sprintf("%s.dcm.state", appName)
}

// Load reads the state file from disk. Returns a new empty state if the file doesn't exist.
func (s *Store) Load(appName string) (*types.State, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return types.NewState(appName), nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var st types.State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &st, nil
}

// Save writes the state to disk.
func (s *Store) Save(st *types.State) error {
	st.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}
	return nil
}
