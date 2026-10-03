// Package events records what happened: an audit trail people read, and an
// outbox of messages for Kafka. Both are written inside the transaction that
// makes the change, so neither can disagree with the data.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/store"
)

// Audit is one entry in the audit trail.
type Audit struct {
	Actor      *uuid.UUID
	Action     string // such as "flag.created"
	ProjectID  *uuid.UUID
	EntityType string // "flag", "environment", "user", ...
	EntityID   *uuid.UUID
	Summary    string // a sentence a person reads
	Data       any    // details, such as the before and after of a change
}

func Record(ctx context.Context, q *store.Queries, a Audit) error {
	data, err := json.Marshal(a.Data)
	if err != nil {
		return err
	}
	if a.Data == nil {
		data = []byte("{}")
	}
	return q.InsertAuditEvent(ctx, store.InsertAuditEventParams{
		ActorID: a.Actor, Action: a.Action, ProjectID: a.ProjectID,
		EntityType: a.EntityType, EntityID: a.EntityID, Summary: a.Summary, Data: data,
	})
}

// FlagChanged is published when a flag's behavior in an environment
// changes: its config, or the flag being archived or restored.
type FlagChanged struct {
	ProjectID     uuid.UUID `json:"project_id"`
	EnvironmentID uuid.UUID `json:"environment_id"`
	FlagKey       string    `json:"flag_key"`
	Version       int32     `json:"version"`
	At            time.Time `json:"at"`
}

// PublishFlagChanged adds a FlagChanged message to the outbox.
func PublishFlagChanged(ctx context.Context, q *store.Queries, m FlagChanged) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return q.InsertOutbox(ctx, store.InsertOutboxParams{
		Topic: kafka.TopicFlagChanges, Key: m.EnvironmentID.String(), Payload: payload,
	})
}

// ExposureBatch is what the SDK API publishes for the worker to count.
type ExposureBatch struct {
	ProjectID     uuid.UUID  `json:"project_id"`
	EnvironmentID uuid.UUID  `json:"environment_id"`
	Exposures     []Exposure `json:"exposures"`
}

type Exposure struct {
	Flag    string    `json:"flag"`
	Variant string    `json:"variant"`
	At      time.Time `json:"at"`
}
