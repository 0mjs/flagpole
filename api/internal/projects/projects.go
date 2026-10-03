// Package projects groups flags: projects, their environments
// (development, staging, production...), and the SDK keys that read an
// environment's flags.
package projects

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/httpx"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/store"
)

type Project struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key" example:"web-app"`
	Name        string    `json:"name" example:"Web app"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type Environment struct {
	ID               uuid.UUID `json:"id"`
	Key              string    `json:"key" example:"production"`
	Name             string    `json:"name" example:"Production"`
	RequiresApproval bool      `json:"requires_approval" doc:"Flag changes here go through a change request another person approves."`
	Position         int       `json:"position"`
}

// ProjectDetail is a project with its environments.
type ProjectDetail struct {
	Project
	Environments []Environment `json:"environments"`
}

type SDKKey struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix" doc:"The start of the key, to tell keys apart."`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func toProject(p store.Project) Project {
	return Project{ID: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, CreatedAt: p.CreatedAt}
}

func ToEnvironment(e store.Environment) Environment {
	return Environment{ID: e.ID, Key: e.Key, Name: e.Name, RequiresApproval: e.RequiresApproval, Position: int(e.Position)}
}

func toSDKKey(k store.SdkKey) SDKKey {
	return SDKKey{ID: k.ID, Name: k.Name, Prefix: k.Prefix, CreatedAt: k.CreatedAt, RevokedAt: k.RevokedAt}
}

type Service struct {
	db *postgres.DB
	// OnKeyRevoked runs after an SDK key is revoked, with its hash, so the
	// SDK API can drop it from its cache.
	OnKeyRevoked func(ctx context.Context, keyHash []byte)
}

func NewService(db *postgres.DB) *Service { return &Service{db: db} }

// defaultEnvironments are created with every project.
var defaultEnvironments = []struct {
	key, name string
	approval  bool
}{
	{"development", "Development", false},
	{"staging", "Staging", false},
	{"production", "Production", true},
}

func (s *Service) List(ctx context.Context) ([]Project, error) {
	rows, err := s.db.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Project, len(rows))
	for i, r := range rows {
		out[i] = toProject(r)
	}
	return out, nil
}

// Create makes a project with development, staging and production
// environments; production requires approval.
func (s *Service) Create(ctx context.Context, actor identity.User, key, name, description string) (ProjectDetail, error) {
	var detail ProjectDetail
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		p, err := q.CreateProject(ctx, store.CreateProjectParams{Key: key, Name: name, Description: description, CreatedBy: &actor.ID})
		if err := keyError(err, "project", key); err != nil {
			return err
		}
		detail.Project = toProject(p)
		for _, e := range defaultEnvironments {
			env, err := q.CreateEnvironment(ctx, store.CreateEnvironmentParams{ProjectID: p.ID, Key: e.key, Name: e.name, RequiresApproval: e.approval})
			if err != nil {
				return err
			}
			detail.Environments = append(detail.Environments, ToEnvironment(env))
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "project.created", ProjectID: &p.ID, EntityType: "project", EntityID: &p.ID,
			Summary: fmt.Sprintf("%s created project %s", actor.Name, name),
		})
	})
	return detail, err
}

// Detail returns the project with its environments.
func (s *Service) Detail(ctx context.Context, p Project) (ProjectDetail, error) {
	envs, err := s.Environments(ctx, p.ID)
	return ProjectDetail{Project: p, Environments: envs}, err
}

func (s *Service) Update(ctx context.Context, actor identity.User, p Project, name, description string) (Project, error) {
	var out Project
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := q.UpdateProject(ctx, store.UpdateProjectParams{ID: p.ID, Name: name, Description: description})
		if err != nil {
			return err
		}
		out = toProject(row)
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "project.updated", ProjectID: &p.ID, EntityType: "project", EntityID: &p.ID,
			Summary: fmt.Sprintf("%s updated project %s", actor.Name, name),
		})
	})
	return out, err
}

func (s *Service) Environments(ctx context.Context, projectID uuid.UUID) ([]Environment, error) {
	rows, err := s.db.ListEnvironments(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Environment, len(rows))
	for i, r := range rows {
		out[i] = ToEnvironment(r)
	}
	return out, nil
}

// Environment returns one of the project's environments by key.
func (s *Service) Environment(ctx context.Context, projectID uuid.UUID, key string) (store.Environment, error) {
	env, err := s.db.GetEnvironment(ctx, store.GetEnvironmentParams{ProjectID: projectID, Key: key})
	if postgres.IsNotFound(err) {
		return env, httpx.NotFound("environment %s not found", key)
	}
	return env, err
}

// CreateEnvironment adds an environment, with every existing flag off.
func (s *Service) CreateEnvironment(ctx context.Context, actor identity.User, p Project, key, name string, requiresApproval bool) (Environment, error) {
	var out Environment
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		env, err := q.CreateEnvironment(ctx, store.CreateEnvironmentParams{ProjectID: p.ID, Key: key, Name: name, RequiresApproval: requiresApproval})
		if err := keyError(err, "environment", key); err != nil {
			return err
		}
		out = ToEnvironment(env)
		if err := q.CreateConfigsForEnvironment(ctx, store.CreateConfigsForEnvironmentParams{
			EnvironmentID: env.ID, ProjectID: p.ID, UpdatedBy: &actor.ID,
		}); err != nil {
			return err
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "environment.created", ProjectID: &p.ID, EntityType: "environment", EntityID: &env.ID,
			Summary: fmt.Sprintf("%s added environment %s to %s", actor.Name, name, p.Name),
		})
	})
	return out, err
}

func (s *Service) UpdateEnvironment(ctx context.Context, actor identity.User, p Project, key, name string, requiresApproval bool) (Environment, error) {
	var out Environment
	err := s.db.Tx(ctx, func(q *store.Queries) error {
		env, err := s.Environment(ctx, p.ID, key)
		if err != nil {
			return err
		}
		row, err := q.UpdateEnvironment(ctx, store.UpdateEnvironmentParams{ID: env.ID, Name: name, RequiresApproval: requiresApproval})
		if err != nil {
			return err
		}
		out = ToEnvironment(row)
		summary := fmt.Sprintf("%s updated environment %s", actor.Name, name)
		if env.RequiresApproval != requiresApproval {
			summary = fmt.Sprintf("%s turned approvals %s for %s", actor.Name, map[bool]string{true: "on", false: "off"}[requiresApproval], name)
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "environment.updated", ProjectID: &p.ID, EntityType: "environment", EntityID: &env.ID,
			Summary: summary, Data: map[string]any{"requires_approval": requiresApproval},
		})
	})
	return out, err
}

func (s *Service) SDKKeys(ctx context.Context, p Project, envKey string) ([]SDKKey, error) {
	env, err := s.Environment(ctx, p.ID, envKey)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.ListSDKKeys(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	out := make([]SDKKey, len(rows))
	for i, r := range rows {
		out[i] = toSDKKey(r)
	}
	return out, nil
}

// CreateSDKKey makes a key and returns it in full, the only time it's shown.
func (s *Service) CreateSDKKey(ctx context.Context, actor identity.User, p Project, envKey, name string) (SDKKey, string, error) {
	env, err := s.Environment(ctx, p.ID, envKey)
	if err != nil {
		return SDKKey{}, "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return SDKKey{}, "", err
	}
	secret := "fp_" + env.Key[:min(3, len(env.Key))] + "_" + base64.RawURLEncoding.EncodeToString(raw)
	var key SDKKey
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := q.CreateSDKKey(ctx, store.CreateSDKKeyParams{
			EnvironmentID: env.ID, Name: name, Prefix: secret[:12], KeyHash: HashSDKKey(secret), CreatedBy: &actor.ID,
		})
		if err != nil {
			return err
		}
		key = toSDKKey(row)
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "sdk_key.created", ProjectID: &p.ID, EntityType: "sdk_key", EntityID: &row.ID,
			Summary: fmt.Sprintf("%s created SDK key %q for %s", actor.Name, name, env.Name),
		})
	})
	return key, secret, err
}

func (s *Service) RevokeSDKKey(ctx context.Context, actor identity.User, p Project, envKey string, id uuid.UUID) error {
	env, err := s.Environment(ctx, p.ID, envKey)
	if err != nil {
		return err
	}
	var hash []byte
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		row, err := q.RevokeSDKKey(ctx, store.RevokeSDKKeyParams{ID: id, EnvironmentID: env.ID})
		if postgres.IsNotFound(err) {
			return httpx.NotFound("active SDK key not found")
		}
		if err != nil {
			return err
		}
		hash = row.KeyHash
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "sdk_key.revoked", ProjectID: &p.ID, EntityType: "sdk_key", EntityID: &row.ID,
			Summary: fmt.Sprintf("%s revoked SDK key %q for %s", actor.Name, row.Name, env.Name),
		})
	})
	if err == nil && s.OnKeyRevoked != nil {
		s.OnKeyRevoked(ctx, hash)
	}
	return err
}

// HashSDKKey is how SDK keys are stored and looked up.
func HashSDKKey(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// keyError turns a duplicate or malformed key into a client error.
func keyError(err error, what, key string) error {
	switch {
	case err == nil:
		return nil
	case postgres.IsUniqueViolation(err):
		return httpx.Conflict("a %s with key %s already exists", what, key)
	case postgres.IsCheckViolation(err):
		return httpx.Invalid(httpx.FieldErrors{"key": "must be lowercase letters, digits and hyphens, 1 to 40 characters"})
	}
	return err
}

type projectKey struct{}

// Load finds the {project} in the path for the rest of the chain, or
// answers 404.
func (s *Service) Load() zinc.HandlerFunc {
	return func(c *zinc.Context) error {
		p, err := s.db.GetProjectByKey(c.Context(), c.Param("project"))
		if postgres.IsNotFound(err) {
			return httpx.NotFound("project %s not found", c.Param("project"))
		}
		if err != nil {
			return err
		}
		c.Set(projectKey{}, toProject(p))
		return c.Next()
	}
}

// Current returns the project Load found.
func Current(c *zinc.Context) Project { return zinc.MustValue[Project](c, projectKey{}) }
