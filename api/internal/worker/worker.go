// Package worker runs the background work: publishing the outbox to Kafka,
// and consuming Kafka to refresh SDK snapshots and count exposures.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/0mjs/flagpole/api/internal/evaluation"
	"github.com/0mjs/flagpole/api/internal/platform/events"
	"github.com/0mjs/flagpole/api/internal/platform/kafka"
	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/store"
)

type Worker struct {
	DB        *postgres.DB
	Snapshots *evaluation.Snapshots
	Producer  *kgo.Client
	Consumer  *kgo.Client
	Log       *slog.Logger
}

// RelayOutbox publishes unpublished outbox rows every interval until ctx
// ends. Rows are claimed with SKIP LOCKED, so running several relays is
// safe, and marked published only after Kafka acknowledges them: a crash in
// between publishes them again, so consumers must tolerate duplicates.
func (w *Worker) RelayOutbox(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for {
			n, err := w.relayBatch(ctx)
			if err != nil && ctx.Err() == nil {
				w.Log.Error("relay outbox", "err", err)
			}
			if n < 100 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) relayBatch(ctx context.Context) (int, error) {
	var n int
	err := w.DB.Tx(ctx, func(q *store.Queries) error {
		rows, err := q.ClaimOutbox(ctx, 100)
		if err != nil || len(rows) == 0 {
			return err
		}
		records := make([]*kgo.Record, len(rows))
		ids := make([]int64, len(rows))
		for i, r := range rows {
			records[i] = &kgo.Record{Topic: r.Topic, Key: []byte(r.Key), Value: r.Payload}
			ids[i] = r.ID
		}
		if err := w.Producer.ProduceSync(ctx, records...).FirstErr(); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		n = len(rows)
		return q.MarkOutboxPublished(ctx, ids)
	})
	return n, err
}

// Consume handles flag changes and exposures until ctx ends, committing
// offsets after each batch is handled.
func (w *Worker) Consume(ctx context.Context) {
	for {
		fetches := w.Consumer.PollFetches(ctx)
		if ctx.Err() != nil {
			return
		}
		fetches.EachError(func(topic string, _ int32, err error) {
			w.Log.Error("kafka fetch", "topic", topic, "err", err)
		})
		var changes []events.FlagChanged
		var exposures []events.ExposureBatch
		fetches.EachRecord(func(r *kgo.Record) {
			switch r.Topic {
			case kafka.TopicFlagChanges:
				var m events.FlagChanged
				if json.Unmarshal(r.Value, &m) == nil {
					changes = append(changes, m)
				}
			case kafka.TopicExposures:
				var m events.ExposureBatch
				if json.Unmarshal(r.Value, &m) == nil {
					exposures = append(exposures, m)
				}
			}
		})
		w.refreshSnapshots(ctx, changes)
		if err := w.countExposures(ctx, exposures); err != nil {
			w.Log.Error("count exposures", "err", err)
			continue // don't commit: the batch is fetched again
		}
		if err := w.Consumer.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			w.Log.Error("commit offsets", "err", err)
		}
	}
}

// refreshSnapshots rebuilds each changed environment's snapshot once per
// batch and tells streaming SDKs about the new version.
func (w *Worker) refreshSnapshots(ctx context.Context, changes []events.FlagChanged) {
	latest := map[uuid.UUID]events.FlagChanged{}
	for _, c := range changes {
		latest[c.EnvironmentID] = c
	}
	for envID, c := range latest {
		snap, err := w.Snapshots.Refresh(ctx, envID)
		if err != nil {
			w.Log.Error("refresh snapshot", "environment", envID, "err", err)
			continue
		}
		if err := w.Snapshots.Announce(ctx, envID, snap.Version, c.FlagKey); err != nil {
			w.Log.Error("announce snapshot", "environment", envID, "err", err)
		}
		w.Log.Info("snapshot refreshed", "environment", envID, "flag", c.FlagKey, "version", snap.Version)
	}
}

type exposureKey struct {
	flag    uuid.UUID
	env     uuid.UUID
	variant string
	bucket  time.Time
}

// countExposures adds the batch's exposures to the hourly counts, in one
// transaction. Exposures for flags that don't exist are dropped.
func (w *Worker) countExposures(ctx context.Context, batches []events.ExposureBatch) error {
	if len(batches) == 0 {
		return nil
	}
	counts := map[exposureKey]int64{}
	flagIDs := map[uuid.UUID]map[string]uuid.UUID{} // project -> flag key -> id
	for _, b := range batches {
		keys := []string{}
		for _, e := range b.Exposures {
			keys = append(keys, e.Flag)
		}
		if flagIDs[b.ProjectID] == nil {
			flagIDs[b.ProjectID] = map[string]uuid.UUID{}
		}
		rows, err := w.DB.FlagIDsByKey(ctx, store.FlagIDsByKeyParams{ProjectID: b.ProjectID, Keys: keys})
		if err != nil {
			return err
		}
		for _, r := range rows {
			flagIDs[b.ProjectID][r.Key] = r.ID
		}
		for _, e := range b.Exposures {
			id, ok := flagIDs[b.ProjectID][e.Flag]
			if !ok {
				continue
			}
			counts[exposureKey{id, b.EnvironmentID, e.Variant, e.At.UTC().Truncate(time.Hour)}]++
		}
	}
	return w.DB.Tx(ctx, func(q *store.Queries) error {
		for k, n := range counts {
			if err := q.AddExposures(ctx, store.AddExposuresParams{
				FlagID: k.flag, EnvironmentID: k.env, Variant: k.variant, Bucket: k.bucket, Count: n,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
