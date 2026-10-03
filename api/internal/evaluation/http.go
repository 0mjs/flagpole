package evaluation

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/0mjs/zinc"

	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/projects"
	"github.com/0mjs/flagpole/api/internal/rules"
)

// API serves the SDK routes.
type API struct {
	db        *postgres.DB
	rdb       *redis.Client
	snapshots *Snapshots
	kafka     *kgo.Client
	log       *slog.Logger
}

func NewAPI(db *postgres.DB, rdb *redis.Client, snapshots *Snapshots, producer *kgo.Client, log *slog.Logger) *API {
	return &API{db: db, rdb: rdb, snapshots: snapshots, kafka: producer, log: log}
}

// Environment is the environment an SDK key reads.
type Environment struct {
	ID         uuid.UUID `json:"environment_id"`
	Key        string    `json:"environment_key"`
	ProjectID  uuid.UUID `json:"project_id"`
	ProjectKey string    `json:"project_key"`
}

type envKey struct{}

// RequireSDKKey checks the Authorization: Bearer key and loads its
// environment. Keys are cached in Redis for a minute; revoking one removes
// it from the cache at once.
func (a *API) RequireSDKKey() zinc.HandlerFunc {
	return func(c *zinc.Context) error {
		key, ok := strings.CutPrefix(c.Header(zinc.HeaderAuthorization), "Bearer ")
		if !ok || !strings.HasPrefix(key, "fp_") {
			return zinc.Unauthorized("send an SDK key as Authorization: Bearer fp_...").
				WithHeader(zinc.HeaderWWWAuthenticate, `Bearer realm="flagpole"`)
		}
		hash := projects.HashSDKKey(key)
		cacheKey := SDKKeyCacheKey(hash)
		var env Environment
		if raw, err := a.rdb.Get(c.Context(), cacheKey).Bytes(); err == nil && json.Unmarshal(raw, &env) == nil {
			c.Set(envKey{}, env)
			return c.Next()
		}
		row, err := a.db.EnvironmentForSDKKey(c.Context(), hash)
		if postgres.IsNotFound(err) {
			return zinc.Unauthorized("unknown or revoked SDK key")
		}
		if err != nil {
			return err
		}
		env = Environment{ID: row.EnvironmentID, Key: row.EnvironmentKey, ProjectID: row.ProjectID, ProjectKey: row.ProjectKey}
		if raw, err := json.Marshal(env); err == nil {
			a.rdb.Set(c.Context(), cacheKey, raw, time.Minute)
		}
		c.Set(envKey{}, env)
		return c.Next()
	}
}

// SDKKeyCacheKey is where a key's environment is cached.
func SDKKeyCacheKey(hash []byte) string { return "sdk-key:" + hex.EncodeToString(hash) }

func current(c *zinc.Context) Environment { return zinc.MustValue[Environment](c, envKey{}) }

// Routes registers the SDK API on sdk, which already requires an SDK key.
func (a *API) Routes(sdk *zinc.Group) {
	sdk.Tags("sdk")
	sdk.Get("/flags", zinc.Typed(a.snapshot)).Name("getSnapshot").
		Summary("Get the environment's flags").
		Description("Everything needed to evaluate locally. Send the last version as If-None-Match to get 304 when nothing changed.").
		Response(http.StatusNotModified, nil).
		Errors(http.StatusUnauthorized)
	sdk.Post("/evaluate", zinc.Typed(a.evaluate)).Name("evaluate").
		Summary("Evaluate flags for a context").
		Description("Evaluates every flag, or the ones listed, on the server. A flag that doesn't exist answers its reason as not_found.").
		Errors(http.StatusUnauthorized).
		RequestExample("a signed-in user", evaluateInput{
			Context: rules.Context{Key: "user-42", Attributes: map[string]any{"country": "GB", "plan": "pro"}},
			Flags:   []string{"new-checkout"},
		})
	sdk.Post("/exposures", zinc.Typed(a.exposures)).Name("reportExposures").Status(http.StatusAccepted).
		Summary("Report exposures").
		Description("Which variants contexts were served, for the dashboard's charts. Batched to Kafka and counted by the worker.").
		Errors(http.StatusUnauthorized)
	sdk.Get("/stream", a.stream).Name("streamChanges").
		Summary("Stream flag changes").
		Description("Server-sent events: ready with the current version, then changed whenever a flag changes. Fetch /flags again on changed.").
		Produces(http.StatusOK, "text/event-stream").
		Errors(http.StatusUnauthorized)
}

type snapshotOutput struct {
	ETag string `header:"ETag" json:"-"`
	Snapshot
}

func (snapshotOutput) OpenAPIName() string { return "Snapshot" }

type snapshotInput struct {
	IfNoneMatch string `header:"If-None-Match"`
}

func (a *API) snapshot(c *zinc.Context, in snapshotInput) (snapshotOutput, error) {
	snap, err := a.snapshots.Get(c.Context(), current(c).ID)
	if err != nil {
		return snapshotOutput{}, err
	}
	etag := `"` + snap.Version + `"`
	if in.IfNoneMatch == etag {
		c.SetHeader(zinc.HeaderETag, etag)
		return snapshotOutput{}, c.Status(http.StatusNotModified).NoContent()
	}
	return snapshotOutput{ETag: etag, Snapshot: snap}, nil
}

type evaluateInput struct {
	Context rules.Context `json:"context"`
	Flags   []string      `json:"flags,omitempty" validate:"max=500" doc:"Only these flags; every flag when left out."`
}

func (evaluateInput) OpenAPIName() string { return "Evaluate" }

type Evaluation struct {
	Version string                  `json:"version"`
	Flags   map[string]rules.Result `json:"flags"`
}

func (a *API) evaluate(c *zinc.Context, in evaluateInput) (Evaluation, error) {
	snap, err := a.snapshots.Get(c.Context(), current(c).ID)
	if err != nil {
		return Evaluation{}, err
	}
	keys := in.Flags
	if keys == nil {
		for key := range snap.Flags {
			keys = append(keys, key)
		}
	}
	out := Evaluation{Version: snap.Version, Flags: make(map[string]rules.Result, len(keys))}
	for _, key := range keys {
		f, ok := snap.Flags[key]
		if !ok {
			out.Flags[key] = rules.Result{Reason: rules.ReasonNotFound, Value: json.RawMessage("null")}
			continue
		}
		out.Flags[key] = rules.Evaluate(key, f.Config, f.Variants, in.Context)
	}
	return out, nil
}

type exposuresInput struct {
	Exposures []exposure `json:"exposures" validate:"required,min=1,max=1000"`
}

func (exposuresInput) OpenAPIName() string { return "ReportExposures" }

type exposure struct {
	Flag    string     `json:"flag" validate:"required,max=80"`
	Variant string     `json:"variant" validate:"required,max=40"`
	At      *time.Time `json:"at,omitempty" doc:"When it was served; now when left out."`
}

func (exposure) OpenAPIName() string { return "Exposure" }

func (a *API) exposures(c *zinc.Context, in exposuresInput) (zinc.NoContent, error) {
	env := current(c)
	batch := events.ExposureBatch{ProjectID: env.ProjectID, EnvironmentID: env.ID, Exposures: make([]events.Exposure, len(in.Exposures))}
	now := time.Now()
	for i, e := range in.Exposures {
		at := now
		if e.At != nil && e.At.Before(now) {
			at = *e.At
		}
		batch.Exposures[i] = events.Exposure{Flag: e.Flag, Variant: e.Variant, At: at}
	}
	value, err := json.Marshal(batch)
	if err != nil {
		return zinc.NoContent{}, err
	}
	// Exposures are analytics: accept them at once and publish in the
	// background, logging a failure rather than failing the SDK. The
	// request's context ends with the response, so the publish mustn't
	// inherit its cancellation.
	a.kafka.Produce(context.WithoutCancel(c.Context()), &kgo.Record{Topic: kafka.TopicExposures, Key: []byte(env.ID.String()), Value: value},
		func(_ *kgo.Record, err error) {
			if err != nil {
				a.log.Error("publish exposures", "environment", env.Key, "err", err)
			}
		})
	return zinc.NoContent{}, nil
}

func (a *API) stream(c *zinc.Context) error {
	env := current(c)
	ctx := c.Context()
	sub := a.rdb.Subscribe(ctx, ChangesChannel(env.ID))
	defer sub.Close()
	snap, err := a.snapshots.Get(ctx, env.ID)
	if err != nil {
		return err
	}
	if err := c.SSE(zinc.Event{Event: "ready", Data: Change{Version: snap.Version}}); err != nil {
		return err
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	messages := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ping.C:
			if err := c.SSE(zinc.Event{Event: "ping", Data: map[string]string{}}); err != nil {
				return nil
			}
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			var change Change
			if json.Unmarshal([]byte(msg.Payload), &change) != nil {
				continue
			}
			if err := c.SSE(zinc.Event{Event: "changed", Data: change}); err != nil {
				return nil
			}
		}
	}
}
