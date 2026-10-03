package projects

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/identity"
)

// Routes registers the project list on projects, and everything about one
// project on project, the group at /projects/{project}.
func (s *Service) Routes(projects, project *zinc.Group) {
	projects.Tags("projects")
	projects.Get("", identity.RequireRole(identity.Viewer), zinc.Typed(s.list)).Name("listProjects").Summary("List projects")
	projects.Post("", identity.RequireRole(identity.Admin), zinc.Typed(s.create)).Name("createProject").Security("session:admin").Status(http.StatusCreated).
		Summary("Create a project").
		Description("Creates it with development, staging and production environments; production requires approval.").
		Errors(http.StatusConflict)

	project.Use(identity.RequireRole(identity.Viewer), s.Load())
	project.Tags("projects")
	project.Get("", zinc.Typed(s.detail)).Name("getProject").Summary("Get a project and its environments").Errors(http.StatusNotFound)
	project.Patch("", identity.RequireRole(identity.Admin), zinc.Typed(s.update)).Name("updateProject").Security("session:admin").Summary("Update a project")

	envs := project.Group("/environments")
	envs.Tags("environments")
	envs.Get("", zinc.Typed(s.environments)).Name("listEnvironments").Summary("List environments")
	envs.Post("", identity.RequireRole(identity.Admin), zinc.Typed(s.createEnvironment)).Name("createEnvironment").Security("session:admin").Status(http.StatusCreated).
		Summary("Add an environment").
		Description("Every existing flag starts off in it.").
		Errors(http.StatusConflict)
	envs.Patch("/{env}", identity.RequireRole(identity.Admin), zinc.Typed(s.updateEnvironment)).Name("updateEnvironment").Security("session:admin").
		Summary("Update an environment").
		Description("Turning on requires_approval makes flag changes here go through change requests.").
		Errors(http.StatusNotFound)

	keys := envs.Group("/{env}/sdk-keys", identity.RequireRole(identity.Admin))
	keys.Tags("sdk-keys").Security("session:admin")
	keys.Get("", zinc.Typed(s.sdkKeys)).Name("listSDKKeys").Summary("List an environment's SDK keys").Errors(http.StatusNotFound)
	keys.Post("", zinc.Typed(s.createSDKKey)).Name("createSDKKey").Status(http.StatusCreated).
		Summary("Create an SDK key").
		Description("The response is the only time the full key is shown; only its hash is stored.").
		Errors(http.StatusNotFound)
	keys.Delete("/{id}", zinc.Typed(s.revokeSDKKey)).Name("revokeSDKKey").Summary("Revoke an SDK key").Errors(http.StatusNotFound)
}

func (s *Service) list(c *zinc.Context, _ struct{}) ([]Project, error) {
	return s.List(c.Context())
}

type createInput struct {
	Key         string `json:"key" validate:"required" pattern:"^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$" doc:"Lowercase letters, digits and hyphens, not starting or ending with a hyphen." example:"web-app"`
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=500"`
}

func (createInput) OpenAPIName() string { return "CreateProject" }

type created struct {
	Location string `header:"Location" json:"-"`
	ProjectDetail
}

func (created) OpenAPIName() string { return "CreatedProject" }

func (s *Service) create(c *zinc.Context, in createInput) (created, error) {
	p, err := s.Create(c.Context(), identity.Actor(c), in.Key, in.Name, in.Description)
	return created{Location: "/api/v1/projects/" + p.Key, ProjectDetail: p}, err
}

func (s *Service) detail(c *zinc.Context, _ struct{}) (ProjectDetail, error) {
	return s.Detail(c.Context(), Current(c))
}

type updateInput struct {
	Name        string `json:"name" validate:"required,max=100"`
	Description string `json:"description" validate:"max=500"`
}

func (updateInput) OpenAPIName() string { return "UpdateProject" }

func (s *Service) update(c *zinc.Context, in updateInput) (Project, error) {
	return s.Update(c.Context(), identity.Actor(c), Current(c), in.Name, in.Description)
}

func (s *Service) environments(c *zinc.Context, _ struct{}) ([]Environment, error) {
	return s.Environments(c.Context(), Current(c).ID)
}

type createEnvironmentInput struct {
	Key              string `json:"key" validate:"required" pattern:"^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$" doc:"Lowercase letters, digits and hyphens, not starting or ending with a hyphen." example:"qa"`
	Name             string `json:"name" validate:"required,max=100"`
	RequiresApproval bool   `json:"requires_approval"`
}

func (createEnvironmentInput) OpenAPIName() string { return "CreateEnvironment" }

func (s *Service) createEnvironment(c *zinc.Context, in createEnvironmentInput) (Environment, error) {
	return s.CreateEnvironment(c.Context(), identity.Actor(c), Current(c), in.Key, in.Name, in.RequiresApproval)
}

type updateEnvironmentInput struct {
	Env              string `path:"env"`
	Name             string `json:"name" validate:"required,max=100"`
	RequiresApproval bool   `json:"requires_approval"`
}

func (updateEnvironmentInput) OpenAPIName() string { return "UpdateEnvironment" }

func (s *Service) updateEnvironment(c *zinc.Context, in updateEnvironmentInput) (Environment, error) {
	return s.UpdateEnvironment(c.Context(), identity.Actor(c), Current(c), in.Env, in.Name, in.RequiresApproval)
}

type envPath struct {
	Env string `path:"env"`
}

func (s *Service) sdkKeys(c *zinc.Context, in envPath) ([]SDKKey, error) {
	return s.SDKKeys(c.Context(), Current(c), in.Env)
}

type createSDKKeyInput struct {
	Env  string `path:"env"`
	Name string `json:"name" validate:"required,max=100" example:"checkout-service"`
}

func (createSDKKeyInput) OpenAPIName() string { return "CreateSDKKey" }

// NewSDKKey is a key as created: the full key, shown this once.
type NewSDKKey struct {
	SDKKey
	Key string `json:"key" doc:"Send as Authorization: Bearer <key> to the SDK API. Shown once."`
}

func (s *Service) createSDKKey(c *zinc.Context, in createSDKKeyInput) (NewSDKKey, error) {
	key, secret, err := s.CreateSDKKey(c.Context(), identity.Actor(c), Current(c), in.Env, in.Name)
	return NewSDKKey{SDKKey: key, Key: secret}, err
}

type sdkKeyPath struct {
	Env string    `path:"env"`
	ID  uuid.UUID `path:"id"`
}

func (s *Service) revokeSDKKey(c *zinc.Context, in sdkKeyPath) (zinc.NoContent, error) {
	return zinc.NoContent{}, s.RevokeSDKKey(c.Context(), identity.Actor(c), Current(c), in.Env, in.ID)
}
