// Package insights is what happened: the audit trail, and how often each
// variant was served, from the exposures SDKs report.
package insights

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/httpx"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/store"
)

type Service struct {
	db       *postgres.DB
	projects *projects.Service
}

func NewService(db *postgres.DB, p *projects.Service) *Service {
	return &Service{db: db, projects: p}
}

type AuditEvent struct {
	ID         int64           `json:"id"`
	Action     string          `json:"action" example:"flag.config_changed"`
	Summary    string          `json:"summary" example:"Ada turned new-checkout on in Staging"`
	EntityType string          `json:"entity_type"`
	EntityID   *uuid.UUID      `json:"entity_id,omitempty"`
	Actor      *Actor          `json:"actor,omitempty"`
	Data       json.RawMessage `json:"data"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Actor struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

// AuditPage is a page of events, newest first.
type AuditPage struct {
	Events []AuditEvent `json:"events"`
	// Before fetches the next, older page; absent on the last page.
	Before *int64 `json:"before,omitempty"`
}

type ExposureBucket struct {
	At     time.Time        `json:"at"`
	Counts map[string]int64 `json:"counts" doc:"By variant."`
}

type Exposures struct {
	Since   time.Time        `json:"since"`
	Until   time.Time        `json:"until"`
	Totals  map[string]int64 `json:"totals" doc:"By variant."`
	Buckets []ExposureBucket `json:"buckets" doc:"One an hour, oldest first; hours with none are left out."`
}

// Routes registers the global audit log on audit and the per-project
// insights on project.
func (s *Service) Routes(audit, project *zinc.Group) {
	audit.Tags("audit")
	audit.Get("", identity.RequireRole(identity.Admin), zinc.Typed(s.allAudit)).Name("listAuditEvents").Security("session:admin").
		Summary("The audit log for everything").
		Description("Includes user changes, which belong to no project. Admins only.")

	project.Get("/audit", zinc.Typed(s.projectAudit)).Name("listProjectAuditEvents").Tags("audit").Summary("A project's audit log")
	project.Get("/flags/{flag}/environments/{env}/exposures", zinc.Typed(s.exposures)).Name("getExposures").Tags("flags").
		Summary("How often each variant was served").
		Description("Hourly counts from the exposures SDKs report, for the last hours (24 by default, up to 720).").
		Errors(http.StatusNotFound)
}

type auditInput struct {
	Before *int64 `query:"before" doc:"The before value from the previous page."`
	Limit  int32  `query:"limit" default:"50" validate:"min=1,max=200"`
}

func (s *Service) allAudit(c *zinc.Context, in auditInput) (AuditPage, error) {
	return s.audit(c, nil, in)
}

func (s *Service) projectAudit(c *zinc.Context, in auditInput) (AuditPage, error) {
	id := projects.Current(c).ID
	return s.audit(c, &id, in)
}

func (s *Service) audit(c *zinc.Context, projectID *uuid.UUID, in auditInput) (AuditPage, error) {
	rows, err := s.db.ListAuditEvents(c.Context(), store.ListAuditEventsParams{ProjectID: projectID, Before: in.Before, MaxRows: in.Limit})
	if err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{Events: make([]AuditEvent, len(rows))}
	for i, r := range rows {
		e := AuditEvent{ID: r.ID, Action: r.Action, Summary: r.Summary, EntityType: r.EntityType, EntityID: r.EntityID, Data: r.Data, CreatedAt: r.CreatedAt}
		if r.ActorID != nil && r.ActorName != nil {
			e.Actor = &Actor{ID: *r.ActorID, Name: *r.ActorName, Email: deref(r.ActorEmail)}
		}
		page.Events[i] = e
	}
	if len(rows) == int(in.Limit) {
		last := rows[len(rows)-1].ID
		page.Before = &last
	}
	return page, nil
}

type exposuresInput struct {
	Flag  string `path:"flag"`
	Env   string `path:"env"`
	Hours int    `query:"hours" default:"24" validate:"min=1,max=720"`
}

func (s *Service) exposures(c *zinc.Context, in exposuresInput) (Exposures, error) {
	p := projects.Current(c)
	env, err := s.projects.Environment(c.Context(), p.ID, in.Env)
	if err != nil {
		return Exposures{}, err
	}
	flag, err := s.db.GetFlag(c.Context(), store.GetFlagParams{ProjectID: p.ID, Key: in.Flag})
	if postgres.IsNotFound(err) {
		return Exposures{}, httpx.NotFound("flag %s not found", in.Flag)
	}
	if err != nil {
		return Exposures{}, err
	}
	until := time.Now().Truncate(time.Hour).Add(time.Hour)
	since := until.Add(-time.Duration(in.Hours) * time.Hour)
	rows, err := s.db.ExposureCounts(c.Context(), store.ExposureCountsParams{FlagID: flag.ID, EnvironmentID: env.ID, Since: since, Until: until})
	if err != nil {
		return Exposures{}, err
	}
	out := Exposures{Since: since, Until: until, Totals: map[string]int64{}, Buckets: []ExposureBucket{}}
	for _, r := range rows {
		if n := len(out.Buckets); n == 0 || !out.Buckets[n-1].At.Equal(r.Bucket) {
			out.Buckets = append(out.Buckets, ExposureBucket{At: r.Bucket, Counts: map[string]int64{}})
		}
		out.Buckets[len(out.Buckets)-1].Counts[r.Variant] += r.Count
		out.Totals[r.Variant] += r.Count
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
