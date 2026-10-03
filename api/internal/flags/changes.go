package flags

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/identity"
	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/httpx"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/rules"
	"github.com/0mjs/flagpole/api/internal/store"
)

// ChangeStatus is where a change request is.
type ChangeStatus string

func (ChangeStatus) Enum() []any {
	return []any{"pending", "applied", "rejected", "cancelled", "conflicted"}
}

// ChangeRequest is a proposed config for an environment that requires
// approval.
type ChangeRequest struct {
	ID            uuid.UUID    `json:"id"`
	Flag          string       `json:"flag"`
	Environment   string       `json:"environment"`
	Status        ChangeStatus `json:"status" doc:"conflicted means the config changed after the request was made, so approving it couldn't apply."`
	Proposed      rules.Config `json:"proposed"`
	BaseVersion   int          `json:"base_version"`
	Comment       string       `json:"comment"`
	Author        Person       `json:"author"`
	Reviewer      *Person      `json:"reviewer,omitempty"`
	ReviewComment string       `json:"review_comment,omitempty"`
	ReviewedAt    *time.Time   `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// RequestChange proposes a config for an environment that requires
// approval. One request can be open per flag and environment.
func (s *Service) RequestChange(ctx context.Context, actor identity.User, p projects.Project, key, envKey string, cfg rules.Config, baseVersion int, comment string) (ChangeRequest, error) {
	env, err := s.projects.Environment(ctx, p.ID, envKey)
	if err != nil {
		return ChangeRequest{}, err
	}
	if !env.RequiresApproval {
		return ChangeRequest{}, httpx.Conflict("%s doesn't require approval; change the config directly", env.Name)
	}
	var id uuid.UUID
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		flag, err := s.flag(ctx, q, p, key)
		if err != nil {
			return err
		}
		if err := s.check(ctx, q, flag, cfg); err != nil {
			return err
		}
		current, err := q.GetFlagConfig(ctx, store.GetFlagConfigParams{FlagID: flag.ID, EnvironmentID: env.ID})
		if err != nil {
			return err
		}
		if int(current.Version) != baseVersion {
			return errStale(current.Version, baseVersion)
		}
		proposed, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		cr, err := q.CreateChangeRequest(ctx, store.CreateChangeRequestParams{
			FlagID: flag.ID, EnvironmentID: env.ID, AuthorID: actor.ID, BaseVersion: int32(baseVersion),
			Proposed: proposed, Comment: comment,
		})
		if postgres.IsUniqueViolation(err) {
			return httpx.Conflict("a change to %s in %s is already waiting for review", key, env.Name)
		}
		if err != nil {
			return err
		}
		id = cr.ID
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "change.requested", ProjectID: &p.ID, EntityType: "change_request", EntityID: &cr.ID,
			Summary: fmt.Sprintf("%s requested a change to %s in %s", actor.Name, key, env.Name),
		})
	})
	if err != nil {
		return ChangeRequest{}, err
	}
	return s.ChangeRequest(ctx, p, id)
}

func (s *Service) ChangeRequest(ctx context.Context, p projects.Project, id uuid.UUID) (ChangeRequest, error) {
	row, err := s.db.GetChangeRequest(ctx, store.GetChangeRequestParams{ID: id, ProjectID: p.ID})
	if postgres.IsNotFound(err) {
		return ChangeRequest{}, httpx.NotFound("change request not found")
	}
	if err != nil {
		return ChangeRequest{}, err
	}
	return toChangeRequest(store.ListChangeRequestsRow(row)), nil
}

// ChangeRequests lists the project's change requests, newest first,
// optionally with one status.
func (s *Service) ChangeRequests(ctx context.Context, p projects.Project, status ChangeStatus) ([]ChangeRequest, error) {
	params := store.ListChangeRequestsParams{ProjectID: p.ID}
	if status != "" {
		params.Status = store.NullChangeStatus{ChangeStatus: store.ChangeStatus(status), Valid: true}
	}
	rows, err := s.db.ListChangeRequests(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]ChangeRequest, len(rows))
	for i, r := range rows {
		out[i] = toChangeRequest(r)
	}
	return out, nil
}

// Approve applies a pending change. The author can't approve their own,
// whatever their role. If the config changed since the request, it's marked
// conflicted and nothing is applied.
func (s *Service) Approve(ctx context.Context, actor identity.User, p projects.Project, id uuid.UUID, comment string) (ChangeRequest, error) {
	cr, err := s.ChangeRequest(ctx, p, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	if cr.Status != "pending" {
		return ChangeRequest{}, httpx.Conflict("this change is %s, not pending", cr.Status)
	}
	if cr.Author.ID == actor.ID {
		return ChangeRequest{}, httpx.Forbidden("you can't approve your own change; ask another approver")
	}
	env, err := s.projects.Environment(ctx, p.ID, cr.Environment)
	if err != nil {
		return ChangeRequest{}, err
	}
	var conflict error
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.ReviewChangeRequest(ctx, store.ReviewChangeRequestParams{
			ID: id, Status: store.ChangeStatusApplied, ReviewerID: &actor.ID, ReviewComment: comment,
		}); postgres.IsNotFound(err) {
			return httpx.Conflict("this change was reviewed by someone else just now")
		} else if err != nil {
			return err
		}
		flag, err := s.flag(ctx, q, p, cr.Flag)
		if err != nil {
			return err
		}
		_, err = s.apply(ctx, q, actor, p, flag, env, cr.Proposed, cr.BaseVersion,
			fmt.Sprintf("(approving %s's change request)", cr.Author.Name))
		return err
	})
	if isConflict(err) {
		// Record the conflict on its own: the applying transaction rolled back.
		conflict = err
		err = s.db.Tx(ctx, func(q *store.Queries) error {
			if _, err := q.ReviewChangeRequest(ctx, store.ReviewChangeRequestParams{
				ID: id, Status: store.ChangeStatusConflicted, ReviewerID: &actor.ID, ReviewComment: comment,
			}); err != nil {
				return err
			}
			return events.Record(ctx, q, events.Audit{
				Actor: &actor.ID, Action: "change.conflicted", ProjectID: &p.ID, EntityType: "change_request", EntityID: &id,
				Summary: fmt.Sprintf("%s's change to %s in %s couldn't apply: the config changed after it was requested", cr.Author.Name, cr.Flag, env.Name),
			})
		})
	}
	if err != nil {
		return ChangeRequest{}, err
	}
	if conflict != nil {
		return ChangeRequest{}, conflict
	}
	s.cache.Invalidate(ctx, env.ID)
	return s.ChangeRequest(ctx, p, id)
}

// Reject closes a pending change without applying it.
func (s *Service) Reject(ctx context.Context, actor identity.User, p projects.Project, id uuid.UUID, comment string) (ChangeRequest, error) {
	return s.close(ctx, actor, p, id, store.ChangeStatusRejected, comment, "rejected")
}

// Cancel withdraws a pending change. Only its author, or an admin, can.
func (s *Service) Cancel(ctx context.Context, actor identity.User, p projects.Project, id uuid.UUID) (ChangeRequest, error) {
	cr, err := s.ChangeRequest(ctx, p, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	if cr.Author.ID != actor.ID && actor.Role != identity.Admin {
		return ChangeRequest{}, httpx.Forbidden("only the author or an admin can cancel a change request")
	}
	return s.close(ctx, actor, p, id, store.ChangeStatusCancelled, "", "cancelled")
}

func (s *Service) close(ctx context.Context, actor identity.User, p projects.Project, id uuid.UUID, status store.ChangeStatus, comment, verb string) (ChangeRequest, error) {
	cr, err := s.ChangeRequest(ctx, p, id)
	if err != nil {
		return ChangeRequest{}, err
	}
	err = s.db.Tx(ctx, func(q *store.Queries) error {
		if _, err := q.ReviewChangeRequest(ctx, store.ReviewChangeRequestParams{
			ID: id, Status: status, ReviewerID: &actor.ID, ReviewComment: comment,
		}); postgres.IsNotFound(err) {
			return httpx.Conflict("this change is %s, not pending", cr.Status)
		} else if err != nil {
			return err
		}
		return events.Record(ctx, q, events.Audit{
			Actor: &actor.ID, Action: "change." + verb, ProjectID: &p.ID, EntityType: "change_request", EntityID: &id,
			Summary: fmt.Sprintf("%s %s %s's change to %s in %s", actor.Name, verb, cr.Author.Name, cr.Flag, cr.Environment),
		})
	})
	if err != nil {
		return ChangeRequest{}, err
	}
	return s.ChangeRequest(ctx, p, id)
}

func toChangeRequest(r store.ListChangeRequestsRow) ChangeRequest {
	cr := ChangeRequest{
		ID: r.ID, Flag: r.FlagKey, Environment: r.EnvironmentKey, Status: ChangeStatus(r.Status),
		BaseVersion: int(r.BaseVersion), Comment: r.Comment, Author: Person{ID: r.AuthorID, Name: r.AuthorName},
		ReviewComment: r.ReviewComment, ReviewedAt: r.ReviewedAt, CreatedAt: r.CreatedAt,
	}
	_ = json.Unmarshal(r.Proposed, &cr.Proposed)
	cr.Proposed.Rules = nonNil(cr.Proposed.Rules)
	if r.ReviewerID != nil && r.ReviewerName != nil {
		cr.Reviewer = &Person{ID: *r.ReviewerID, Name: *r.ReviewerName}
	}
	return cr
}

func isConflict(err error) bool {
	var h *zinc.HTTPError
	return errors.As(err, &h) && h.Code == http.StatusConflict
}
