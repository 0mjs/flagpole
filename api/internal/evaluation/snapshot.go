// Package evaluation is the SDK API: what services call, with an SDK key, to
// get flag values. It reads environment snapshots from Redis, built from
// Postgres when missing, and streams changes as they happen.
package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/0mjs/flagpole/api/internal/platform/postgres"
	"github.com/0mjs/flagpole/api/internal/rules"
)

// Snapshot is everything needed to evaluate one environment's flags, so an
// SDK can evaluate locally and only fetch again when Version changes.
type Snapshot struct {
	Version string                  `json:"version" doc:"Changes whenever any flag in the environment changes. Also sent as the ETag."`
	Flags   map[string]SnapshotFlag `json:"flags" doc:"By flag key. Archived flags are left out."`
}

type SnapshotFlag struct {
	Kind     string                     `json:"kind"`
	Version  int                        `json:"version"`
	Variants map[string]json.RawMessage `json:"variants"`
	rules.Config
}

// Snapshots caches environment snapshots in Redis.
type Snapshots struct {
	db  *postgres.DB
	rdb *redis.Client
}

func NewSnapshots(db *postgres.DB, rdb *redis.Client) *Snapshots {
	return &Snapshots{db: db, rdb: rdb}
}

func snapshotKey(envID uuid.UUID) string { return "snapshot:" + envID.String() }

// ChangesChannel is the Redis channel announcing an environment's new
// snapshot version.
func ChangesChannel(envID uuid.UUID) string { return "snapshot-changes:" + envID.String() }

// Get returns the cached snapshot, building and caching it on a miss.
func (s *Snapshots) Get(ctx context.Context, envID uuid.UUID) (Snapshot, error) {
	if raw, err := s.rdb.Get(ctx, snapshotKey(envID)).Bytes(); err == nil {
		var snap Snapshot
		if json.Unmarshal(raw, &snap) == nil {
			return snap, nil
		}
	}
	return s.Refresh(ctx, envID)
}

// Refresh rebuilds the snapshot from Postgres and caches it.
func (s *Snapshots) Refresh(ctx context.Context, envID uuid.UUID) (Snapshot, error) {
	rows, err := s.db.EnvironmentSnapshot(ctx, envID)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{Flags: make(map[string]SnapshotFlag, len(rows))}
	for _, r := range rows {
		f := SnapshotFlag{Kind: string(r.Kind), Version: int(r.Version),
			Config: rules.Config{Enabled: r.Enabled, DefaultVariant: r.DefaultVariant, OffVariant: r.OffVariant}}
		_ = json.Unmarshal(r.Rules, &f.Rules)
		_ = json.Unmarshal(r.Variants, &f.Variants)
		snap.Flags[r.Key] = f
	}
	// The version is a hash of the content, so it changes exactly when the
	// snapshot does, however it was rebuilt.
	content, err := json.Marshal(snap.Flags)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(content)
	snap.Version = hex.EncodeToString(sum[:8])
	if raw, err := json.Marshal(snap); err == nil {
		s.rdb.Set(ctx, snapshotKey(envID), raw, time.Hour)
	}
	return snap, nil
}

// Invalidate drops the cached snapshot; the next Get rebuilds it.
func (s *Snapshots) Invalidate(ctx context.Context, envID uuid.UUID) {
	s.rdb.Del(ctx, snapshotKey(envID))
}

// Announce tells streaming SDKs the environment has a new version.
func (s *Snapshots) Announce(ctx context.Context, envID uuid.UUID, version, flag string) error {
	msg, _ := json.Marshal(Change{Version: version, Flag: flag})
	return s.rdb.Publish(ctx, ChangesChannel(envID), msg).Err()
}

// Change is the event the stream sends when a flag changes.
type Change struct {
	Version string `json:"version"`
	Flag    string `json:"flag"`
}
