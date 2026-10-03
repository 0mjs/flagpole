-- name: InsertAuditEvent :exec
INSERT INTO audit_events (actor_id, action, project_id, entity_type, entity_id, summary, data)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAuditEvents :many
SELECT ae.*, u.name AS actor_name, u.email AS actor_email
FROM audit_events ae
LEFT JOIN users u ON u.id = ae.actor_id
WHERE (sqlc.narg(project_id)::uuid IS NULL OR ae.project_id = sqlc.narg(project_id))
  AND (sqlc.narg(before)::bigint IS NULL OR ae.id < sqlc.narg(before))
ORDER BY ae.id DESC
LIMIT sqlc.arg(max_rows);

-- name: InsertOutbox :exec
INSERT INTO outbox (topic, key, payload) VALUES ($1, $2, $3);

-- name: ClaimOutbox :many
-- SKIP LOCKED lets several relays run without waiting on each other.
SELECT * FROM outbox
WHERE published_at IS NULL
ORDER BY id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox SET published_at = now() WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: AddExposures :exec
INSERT INTO exposure_counts (flag_id, environment_id, variant, bucket, count)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (flag_id, environment_id, bucket, variant)
DO UPDATE SET count = exposure_counts.count + EXCLUDED.count;

-- name: ExposureCounts :many
SELECT bucket, variant, count FROM exposure_counts
WHERE flag_id = $1 AND environment_id = $2 AND bucket >= sqlc.arg(since) AND bucket < sqlc.arg(until)
ORDER BY bucket, variant;
