-- name: CreateFlag :one
INSERT INTO flags (project_id, key, name, description, kind, tags, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFlag :one
SELECT * FROM flags WHERE project_id = $1 AND key = $2;

-- name: ListFlags :many
SELECT * FROM flags
WHERE project_id = $1 AND (sqlc.arg(include_archived)::boolean OR archived_at IS NULL)
ORDER BY key;

-- name: UpdateFlag :one
UPDATE flags SET name = $2, description = $3, tags = $4, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetFlagArchived :one
UPDATE flags
SET archived_at = CASE WHEN sqlc.arg(archived)::boolean THEN now() END, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateVariant :one
INSERT INTO flag_variants (flag_id, key, value, description, position)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListVariants :many
SELECT * FROM flag_variants WHERE flag_id = ANY(sqlc.arg(flag_ids)::uuid[]) ORDER BY flag_id, position;

-- name: CreateFlagConfig :one
INSERT INTO flag_configs (flag_id, environment_id, enabled, default_variant, off_variant, rules, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetFlagConfig :one
SELECT * FROM flag_configs WHERE flag_id = $1 AND environment_id = $2;

-- name: ListFlagConfigs :many
SELECT c.*, e.key AS environment_key
FROM flag_configs c
JOIN environments e ON e.id = c.environment_id
WHERE c.flag_id = ANY(sqlc.arg(flag_ids)::uuid[])
ORDER BY e.position, e.key;

-- name: UpdateFlagConfig :one
-- Applies only if the config is still at the version the change was based on.
UPDATE flag_configs
SET enabled = $3, default_variant = $4, off_variant = $5, rules = $6,
    version = version + 1, updated_by = $7, updated_at = now()
WHERE flag_id = $1 AND environment_id = $2 AND version = sqlc.arg(base_version)
RETURNING *;

-- name: EnvironmentSnapshot :many
-- Everything an SDK needs to evaluate one environment's flags.
SELECT f.id, f.key, f.kind, c.enabled, c.default_variant, c.off_variant, c.rules, c.version,
       (SELECT jsonb_object_agg(v.key, v.value) FROM flag_variants v WHERE v.flag_id = f.id)::jsonb AS variants
FROM flags f
JOIN flag_configs c ON c.flag_id = f.id
WHERE c.environment_id = $1 AND f.archived_at IS NULL
ORDER BY f.key;

-- name: FlagIDsByKey :many
SELECT id, key FROM flags WHERE project_id = $1 AND key = ANY(sqlc.arg(keys)::text[]);
