package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides persistence for applications, policies, deployments, and history.
type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at the given path and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS applications (
			name       TEXT PRIMARY KEY,
			labels     TEXT NOT NULL DEFAULT '{}',
			components TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS policies (
			name       TEXT PRIMARY KEY,
			rules      TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS deployments (
			id               TEXT PRIMARY KEY,
			application_name TEXT NOT NULL REFERENCES applications(name),
			status           TEXT NOT NULL DEFAULT 'pending',
			plan             TEXT,
			state            TEXT,
			error            TEXT,
			policies         TEXT NOT NULL DEFAULT '[]',
			created_at       TEXT NOT NULL,
			updated_at       TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS deployment_history (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id TEXT NOT NULL REFERENCES deployments(id),
			action        TEXT NOT NULL,
			details       TEXT,
			created_at    TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS environments (
			name       TEXT PRIMARY KEY,
			provider   TEXT NOT NULL,
			labels     TEXT NOT NULL DEFAULT '{}',
			config     TEXT NOT NULL DEFAULT '{}',
			resources  TEXT,
			cost       TEXT,
			status     TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_deployments_app ON deployments(application_name);
		CREATE INDEX IF NOT EXISTS idx_history_deployment ON deployment_history(deployment_id);
		CREATE INDEX IF NOT EXISTS idx_environments_provider ON environments(provider);
	`)
	return err
}

// --- Application CRUD ---

type ApplicationRecord struct {
	Name       string            `json:"name"`
	Labels     map[string]string `json:"labels"`
	Components json.RawMessage   `json:"components"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

func (s *Store) CreateApplication(app *ApplicationRecord) error {
	labels, _ := json.Marshal(app.Labels)
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO applications (name, labels, components, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		app.Name, string(labels), string(app.Components), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting application: %w", err)
	}
	app.CreatedAt = now
	app.UpdatedAt = now
	return nil
}

func (s *Store) GetApplication(name string) (*ApplicationRecord, error) {
	row := s.db.QueryRow(
		`SELECT name, labels, components, created_at, updated_at FROM applications WHERE name = ?`, name,
	)
	return scanApplication(row)
}

func (s *Store) ListApplications() ([]ApplicationRecord, error) {
	rows, err := s.db.Query(
		`SELECT name, labels, components, created_at, updated_at FROM applications ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []ApplicationRecord
	for rows.Next() {
		app, err := scanApplicationRow(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, *app)
	}
	return apps, rows.Err()
}

func (s *Store) UpdateApplication(app *ApplicationRecord) error {
	labels, _ := json.Marshal(app.Labels)
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE applications SET labels = ?, components = ?, updated_at = ? WHERE name = ?`,
		string(labels), string(app.Components), now.Format(time.RFC3339), app.Name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	app.UpdatedAt = now
	return nil
}

func (s *Store) DeleteApplication(name string) error {
	// Check for active deployments.
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM deployments WHERE application_name = ? AND status NOT IN ('destroyed', 'failed', 'planned')`,
		name,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete application %q: has %d active deployment(s)", name, count)
	}

	res, err := s.db.Exec(`DELETE FROM applications WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Policy CRUD ---

type PolicyRecord struct {
	Name      string          `json:"name"`
	Rules     json.RawMessage `json:"rules"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

func (s *Store) CreatePolicy(p *PolicyRecord) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO policies (name, rules, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		p.Name, string(p.Rules), now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting policy: %w", err)
	}
	p.CreatedAt = now
	p.UpdatedAt = now
	return nil
}

func (s *Store) GetPolicy(name string) (*PolicyRecord, error) {
	row := s.db.QueryRow(
		`SELECT name, rules, created_at, updated_at FROM policies WHERE name = ?`, name,
	)
	return scanPolicy(row)
}

func (s *Store) ListPolicies() ([]PolicyRecord, error) {
	rows, err := s.db.Query(
		`SELECT name, rules, created_at, updated_at FROM policies ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []PolicyRecord
	for rows.Next() {
		p, err := scanPolicyRow(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, *p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdatePolicy(p *PolicyRecord) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE policies SET rules = ?, updated_at = ? WHERE name = ?`,
		string(p.Rules), now.Format(time.RFC3339), p.Name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	p.UpdatedAt = now
	return nil
}

func (s *Store) DeletePolicy(name string) error {
	res, err := s.db.Exec(`DELETE FROM policies WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Deployment CRUD ---

type DeploymentRecord struct {
	ID              string          `json:"id"`
	ApplicationName string          `json:"application"`
	Status          string          `json:"status"`
	Plan            json.RawMessage `json:"plan,omitempty"`
	State           json.RawMessage `json:"state,omitempty"`
	Error           string          `json:"error,omitempty"`
	Policies        []string        `json:"policies"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

func (s *Store) CreateDeployment(d *DeploymentRecord) error {
	now := time.Now().UTC()
	policies, _ := json.Marshal(d.Policies)
	_, err := s.db.Exec(
		`INSERT INTO deployments (id, application_name, status, policies, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		d.ID, d.ApplicationName, d.Status, string(policies),
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	d.CreatedAt = now
	d.UpdatedAt = now
	return nil
}

func (s *Store) GetDeployment(id string) (*DeploymentRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, application_name, status, plan, state, error, policies, created_at, updated_at
		 FROM deployments WHERE id = ?`, id,
	)
	return scanDeployment(row)
}

func (s *Store) ListDeployments() ([]DeploymentRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, application_name, status, plan, state, error, policies, created_at, updated_at
		 FROM deployments ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []DeploymentRecord
	for rows.Next() {
		d, err := scanDeploymentRow(rows)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, *d)
	}
	return deployments, rows.Err()
}

func (s *Store) UpdateDeployment(d *DeploymentRecord) error {
	now := time.Now().UTC()
	var planStr, stateStr *string
	if d.Plan != nil {
		s := string(d.Plan)
		planStr = &s
	}
	if d.State != nil {
		s := string(d.State)
		stateStr = &s
	}

	_, err := s.db.Exec(
		`UPDATE deployments SET status = ?, plan = ?, state = ?, error = ?, updated_at = ? WHERE id = ?`,
		d.Status, planStr, stateStr, d.Error, now.Format(time.RFC3339), d.ID,
	)
	d.UpdatedAt = now
	return err
}

func (s *Store) DeleteDeployment(id string) error {
	// Delete history first, then deployment.
	s.db.Exec(`DELETE FROM deployment_history WHERE deployment_id = ?`, id)
	res, err := s.db.Exec(`DELETE FROM deployments WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Deployment History ---

type HistoryRecord struct {
	ID           int64           `json:"id"`
	DeploymentID string          `json:"deploymentId"`
	Action       string          `json:"action"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (s *Store) AddHistory(h *HistoryRecord) error {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO deployment_history (deployment_id, action, details, created_at) VALUES (?, ?, ?, ?)`,
		h.DeploymentID, h.Action, string(h.Details), now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}
	h.ID, _ = res.LastInsertId()
	h.CreatedAt = now
	return nil
}

func (s *Store) GetHistory(deploymentID string) ([]HistoryRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, deployment_id, action, details, created_at
		 FROM deployment_history WHERE deployment_id = ? ORDER BY created_at ASC`, deploymentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryRecord
	for rows.Next() {
		var h HistoryRecord
		var createdAt, details string
		if err := rows.Scan(&h.ID, &h.DeploymentID, &h.Action, &details, &createdAt); err != nil {
			return nil, err
		}
		h.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if details != "" {
			h.Details = json.RawMessage(details)
		}
		history = append(history, h)
	}
	return history, rows.Err()
}

// --- Environment CRUD ---

type EnvironmentRecord struct {
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Labels    map[string]string `json:"labels,omitempty"`
	Config    map[string]any    `json:"config,omitempty"`
	Resources json.RawMessage   `json:"resources,omitempty"`
	Cost      json.RawMessage   `json:"cost,omitempty"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

func (s *Store) CreateEnvironment(env *EnvironmentRecord) error {
	labels, _ := json.Marshal(env.Labels)
	config, _ := json.Marshal(env.Config)
	now := time.Now().UTC()
	if env.Status == "" {
		env.Status = "active"
	}
	var resources, cost *string
	if env.Resources != nil {
		r := string(env.Resources)
		resources = &r
	}
	if env.Cost != nil {
		c := string(env.Cost)
		cost = &c
	}
	_, err := s.db.Exec(
		`INSERT INTO environments (name, provider, labels, config, resources, cost, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.Name, env.Provider, string(labels), string(config), resources, cost, env.Status,
		now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("inserting environment: %w", err)
	}
	env.CreatedAt = now
	env.UpdatedAt = now
	return nil
}

func (s *Store) GetEnvironment(name string) (*EnvironmentRecord, error) {
	row := s.db.QueryRow(
		`SELECT name, provider, labels, config, resources, cost, status, created_at, updated_at
		 FROM environments WHERE name = ?`, name,
	)
	return scanEnvironment(row)
}

func (s *Store) ListEnvironments() ([]EnvironmentRecord, error) {
	rows, err := s.db.Query(
		`SELECT name, provider, labels, config, resources, cost, status, created_at, updated_at
		 FROM environments ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []EnvironmentRecord
	for rows.Next() {
		env, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		envs = append(envs, *env)
	}
	return envs, rows.Err()
}

func (s *Store) UpdateEnvironment(env *EnvironmentRecord) error {
	labels, _ := json.Marshal(env.Labels)
	config, _ := json.Marshal(env.Config)
	now := time.Now().UTC()
	var resources, cost *string
	if env.Resources != nil {
		r := string(env.Resources)
		resources = &r
	}
	if env.Cost != nil {
		c := string(env.Cost)
		cost = &c
	}
	res, err := s.db.Exec(
		`UPDATE environments SET provider = ?, labels = ?, config = ?, resources = ?, cost = ?, status = ?, updated_at = ?
		 WHERE name = ?`,
		env.Provider, string(labels), string(config), resources, cost, env.Status, now.Format(time.RFC3339), env.Name,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	env.UpdatedAt = now
	return nil
}

func (s *Store) DeleteEnvironment(name string) error {
	res, err := s.db.Exec(`DELETE FROM environments WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Errors ---

var ErrNotFound = fmt.Errorf("not found")

// --- Scan helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanApplication(row scanner) (*ApplicationRecord, error) {
	var app ApplicationRecord
	var labels, components, createdAt, updatedAt string
	err := row.Scan(&app.Name, &labels, &components, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	json.Unmarshal([]byte(labels), &app.Labels)
	app.Components = json.RawMessage(components)
	app.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	app.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &app, nil
}

func scanApplicationRow(rows *sql.Rows) (*ApplicationRecord, error) {
	return scanApplication(rows)
}

func scanPolicy(row scanner) (*PolicyRecord, error) {
	var p PolicyRecord
	var rules, createdAt, updatedAt string
	err := row.Scan(&p.Name, &rules, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.Rules = json.RawMessage(rules)
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &p, nil
}

func scanPolicyRow(rows *sql.Rows) (*PolicyRecord, error) {
	return scanPolicy(rows)
}

func scanDeployment(row scanner) (*DeploymentRecord, error) {
	var d DeploymentRecord
	var plan, state, errStr, policies, createdAt, updatedAt sql.NullString
	err := row.Scan(&d.ID, &d.ApplicationName, &d.Status, &plan, &state, &errStr, &policies, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if plan.Valid {
		d.Plan = json.RawMessage(plan.String)
	}
	if state.Valid {
		d.State = json.RawMessage(state.String)
	}
	if errStr.Valid {
		d.Error = errStr.String
	}
	if policies.Valid {
		json.Unmarshal([]byte(policies.String), &d.Policies)
	}
	if createdAt.Valid {
		d.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		d.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}
	return &d, nil
}

func scanDeploymentRow(rows *sql.Rows) (*DeploymentRecord, error) {
	return scanDeployment(rows)
}

func scanEnvironment(row scanner) (*EnvironmentRecord, error) {
	var env EnvironmentRecord
	var labels, config, createdAt, updatedAt string
	var resources, cost sql.NullString
	err := row.Scan(&env.Name, &env.Provider, &labels, &config, &resources, &cost, &env.Status, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	json.Unmarshal([]byte(labels), &env.Labels)
	json.Unmarshal([]byte(config), &env.Config)
	if resources.Valid {
		env.Resources = json.RawMessage(resources.String)
	}
	if cost.Valid {
		env.Cost = json.RawMessage(cost.String)
	}
	env.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	env.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &env, nil
}
