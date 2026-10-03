package flags

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/rules"
)

// Routes registers flags and change requests on project, the group at
// /projects/{project}, which has already loaded the project and required a
// signed-in viewer.
func (s *Service) Routes(project *zinc.Group) {
	editor := identity.RequireRole(identity.Editor)
	approver := identity.RequireRole(identity.Approver)

	flags := project.Group("/flags")
	flags.Tags("flags")
	flags.Get("", zinc.Typed(s.list)).Name("listFlags").Summary("List flags").
		Description("Each flag with whether it's on, and how many rules it has, in every environment.")
	flags.Post("", editor, zinc.Typed(s.create)).Name("createFlag").Security("session:editor").Status(http.StatusCreated).
		Summary("Create a flag").
		Description(`Creates the flag off in every environment. A boolean flag gets the variants "on" and "off"; other kinds list theirs, the first being what's served when off.`).
		Errors(http.StatusConflict).
		RequestExample("a boolean flag", createInput{Key: "new-checkout", Name: "New checkout", Kind: "boolean", Tags: []string{"checkout"}}).
		RequestExample("a string flag", createInput{Key: "button-color", Name: "Button color", Kind: "string", Variants: []Variant{
			{Key: "blue", Value: []byte(`"blue"`)}, {Key: "green", Value: []byte(`"green"`)},
		}})
	flags.Get("/{flag}", zinc.Typed(s.get)).Name("getFlag").Summary("Get a flag").Errors(http.StatusNotFound)
	flags.Patch("/{flag}", editor, zinc.Typed(s.update)).Name("updateFlag").Security("session:editor").Summary("Update a flag's name, description or tags").Errors(http.StatusNotFound)
	flags.Post("/{flag}/archive", editor, zinc.Typed(s.archive)).Name("archiveFlag").Security("session:editor").Summary("Archive a flag").
		Description("Takes it out of every SDK snapshot; SDKs then fall back to their own defaults.").Errors(http.StatusNotFound)
	flags.Post("/{flag}/restore", editor, zinc.Typed(s.restore)).Name("restoreFlag").Security("session:editor").Summary("Restore an archived flag").Errors(http.StatusNotFound)

	flags.Get("/{flag}/environments/{env}", zinc.Typed(s.config)).Name("getFlagConfig").Summary("Get a flag's config in an environment").Errors(http.StatusNotFound)
	flags.Put("/{flag}/environments/{env}", editor, zinc.Typed(s.updateConfig)).Name("updateFlagConfig").Security("session:editor").
		Summary("Change a flag's config in an environment").
		Description("For environments that don't require approval. Send the version you loaded as base_version: if someone changed the config since, the answer is 409 and nothing changes.").
		Errors(http.StatusNotFound, http.StatusConflict)
	flags.Post("/{flag}/environments/{env}/change-requests", editor, zinc.Typed(s.requestChange)).Name("requestChange").Security("session:editor").Status(http.StatusCreated).
		Summary("Request a change in an environment that requires approval").
		Description("Another approver, not you, approves it, which applies it.").
		Errors(http.StatusNotFound, http.StatusConflict)

	changes := project.Group("/change-requests")
	changes.Tags("change-requests")
	changes.Get("", zinc.Typed(s.changeRequests)).Name("listChangeRequests").Summary("List change requests").Description("Newest first, up to 200.")
	changes.Get("/{id}", zinc.Typed(s.changeRequest)).Name("getChangeRequest").Summary("Get a change request").Errors(http.StatusNotFound)
	changes.Post("/{id}/approve", approver, zinc.Typed(s.approve)).Name("approveChangeRequest").Security("session:approver").
		Summary("Approve and apply a change").
		Description("You can't approve your own change. If the config changed after the request was made, the request is marked conflicted, nothing is applied, and the answer is 409.").
		Errors(http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
	changes.Post("/{id}/reject", approver, zinc.Typed(s.reject)).Name("rejectChangeRequest").Security("session:approver").Summary("Reject a change").Errors(http.StatusNotFound, http.StatusConflict)
	changes.Post("/{id}/cancel", editor, zinc.Typed(s.cancel)).Name("cancelChangeRequest").Security("session:editor").Summary("Withdraw your change").
		Description("Its author, or an admin, can withdraw it.").Errors(http.StatusForbidden, http.StatusNotFound, http.StatusConflict)
}

type listInput struct {
	IncludeArchived bool `query:"include_archived"`
}

func (s *Service) list(c *zinc.Context, in listInput) ([]Summary, error) {
	return s.List(c.Context(), projects.Current(c), in.IncludeArchived)
}

type createInput struct {
	Key         string    `json:"key" validate:"required" pattern:"^[a-z0-9][a-z0-9._-]{0,79}$" doc:"Lowercase letters and digits, then also dots, hyphens and underscores." example:"new-checkout"`
	Name        string    `json:"name" validate:"required,max=100"`
	Description string    `json:"description,omitempty" validate:"max=500"`
	Kind        Kind      `json:"kind" validate:"required"`
	Tags        []string  `json:"tags,omitempty" validate:"max=20"`
	Variants    []Variant `json:"variants,omitempty" validate:"max=20" doc:"Required for string, number and json flags; leave out for boolean flags."`
}

func (createInput) OpenAPIName() string { return "CreateFlag" }

type createdFlag struct {
	Location string `header:"Location" json:"-"`
	Flag
}

func (createdFlag) OpenAPIName() string { return "CreatedFlag" }

func (s *Service) create(c *zinc.Context, in createInput) (createdFlag, error) {
	p := projects.Current(c)
	flag, err := s.Create(c.Context(), identity.Actor(c), p, NewFlag(in))
	return createdFlag{Location: "/api/v1/projects/" + p.Key + "/flags/" + flag.Key, Flag: flag}, err
}

type flagPath struct {
	Flag string `path:"flag"`
}

func (s *Service) get(c *zinc.Context, in flagPath) (Flag, error) {
	return s.Get(c.Context(), projects.Current(c), in.Flag)
}

type updateInput struct {
	Flag        string   `path:"flag"`
	Name        *string  `json:"name,omitempty" validate:"omitempty,max=100"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Tags        []string `json:"tags,omitempty" validate:"max=20"`
}

func (updateInput) OpenAPIName() string { return "UpdateFlag" }

func (s *Service) update(c *zinc.Context, in updateInput) (Flag, error) {
	return s.Update(c.Context(), identity.Actor(c), projects.Current(c), in.Flag, in.Name, in.Description, in.Tags)
}

func (s *Service) archive(c *zinc.Context, in flagPath) (Flag, error) {
	return s.SetArchived(c.Context(), identity.Actor(c), projects.Current(c), in.Flag, true)
}

func (s *Service) restore(c *zinc.Context, in flagPath) (Flag, error) {
	return s.SetArchived(c.Context(), identity.Actor(c), projects.Current(c), in.Flag, false)
}

type configPath struct {
	Flag string `path:"flag"`
	Env  string `path:"env"`
}

func (s *Service) config(c *zinc.Context, in configPath) (EnvironmentConfig, error) {
	return s.Config(c.Context(), projects.Current(c), in.Flag, in.Env)
}

type configInput struct {
	Flag string `path:"flag"`
	Env  string `path:"env"`
	rules.Config
	BaseVersion int `json:"base_version" validate:"required,min=1" doc:"The version you loaded."`
}

func (configInput) OpenAPIName() string { return "FlagConfig" }

func (s *Service) updateConfig(c *zinc.Context, in configInput) (EnvironmentConfig, error) {
	return s.UpdateConfig(c.Context(), identity.Actor(c), projects.Current(c), in.Flag, in.Env, in.Config, in.BaseVersion)
}

type changeInput struct {
	configInput
	Comment string `json:"comment,omitempty" validate:"max=500" doc:"Why, for the reviewer."`
}

func (changeInput) OpenAPIName() string { return "RequestChange" }

type createdChange struct {
	Location string `header:"Location" json:"-"`
	ChangeRequest
}

func (createdChange) OpenAPIName() string { return "CreatedChangeRequest" }

func (s *Service) requestChange(c *zinc.Context, in changeInput) (createdChange, error) {
	p := projects.Current(c)
	cr, err := s.RequestChange(c.Context(), identity.Actor(c), p, in.Flag, in.Env, in.Config, in.BaseVersion, in.Comment)
	return createdChange{Location: "/api/v1/projects/" + p.Key + "/change-requests/" + cr.ID.String(), ChangeRequest: cr}, err
}

type changesInput struct {
	Status ChangeStatus `query:"status"`
}

func (s *Service) changeRequests(c *zinc.Context, in changesInput) ([]ChangeRequest, error) {
	return s.ChangeRequests(c.Context(), projects.Current(c), in.Status)
}

type changePath struct {
	ID uuid.UUID `path:"id"`
}

func (s *Service) changeRequest(c *zinc.Context, in changePath) (ChangeRequest, error) {
	return s.ChangeRequest(c.Context(), projects.Current(c), in.ID)
}

type reviewInput struct {
	ID      uuid.UUID `path:"id"`
	Comment string    `json:"comment,omitempty" validate:"max=500"`
}

func (reviewInput) OpenAPIName() string { return "Review" }

func (s *Service) approve(c *zinc.Context, in reviewInput) (ChangeRequest, error) {
	return s.Approve(c.Context(), identity.Actor(c), projects.Current(c), in.ID, in.Comment)
}

func (s *Service) reject(c *zinc.Context, in reviewInput) (ChangeRequest, error) {
	return s.Reject(c.Context(), identity.Actor(c), projects.Current(c), in.ID, in.Comment)
}

func (s *Service) cancel(c *zinc.Context, in changePath) (ChangeRequest, error) {
	return s.Cancel(c.Context(), identity.Actor(c), projects.Current(c), in.ID)
}
