-- name: CreateProject :one
INSERT INTO projects (key, name, description, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetProjectByKey :one
SELECT * FROM projects WHERE key = $1;

-- name: ListProjects :many
SELECT * FROM projects ORDER BY name;

-- name: UpdateProject :one
UPDATE projects SET name = $2, description = $3 WHERE id = $1 RETURNING *;

-- name: CreateEnvironment :one
INSERT INTO environments (project_id, key, name, requires_approval, position)
VALUES ($1, $2, $3, $4, (SELECT coalesce(max(position), -1) + 1 FROM environments WHERE project_id = $1))
RETURNING *;

-- name: ListEnvironments :many
SELECT * FROM environments WHERE project_id = $1 ORDER BY position, key;

-- name: GetEnvironment :one
SELECT * FROM environments WHERE project_id = $1 AND key = $2;

-- name: UpdateEnvironment :one
UPDATE environments SET name = $2, requires_approval = $3 WHERE id = $1 RETURNING *;

-- name: CreateSDKKey :one
INSERT INTO sdk_keys (environment_id, name, prefix, key_hash, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListSDKKeys :many
SELECT * FROM sdk_keys WHERE environment_id = $1 ORDER BY created_at DESC;

-- name: RevokeSDKKey :one
UPDATE sdk_keys SET revoked_at = now()
WHERE id = $1 AND environment_id = $2 AND revoked_at IS NULL
RETURNING *;

-- name: EnvironmentForSDKKey :one
SELECT e.id AS environment_id, e.key AS environment_key, p.id AS project_id, p.key AS project_key
FROM sdk_keys k
JOIN environments e ON e.id = k.environment_id
JOIN projects p ON p.id = e.project_id
WHERE k.key_hash = $1 AND k.revoked_at IS NULL;

-- name: CreateConfigsForEnvironment :exec
-- A new environment starts with every flag off, serving the same variants
-- as the project's first environment.
INSERT INTO flag_configs (flag_id, environment_id, enabled, default_variant, off_variant, updated_by)
SELECT DISTINCT ON (c.flag_id) c.flag_id, sqlc.arg(environment_id)::uuid, false, c.default_variant, c.off_variant, sqlc.narg(updated_by)::uuid
FROM flag_configs c
JOIN environments e ON e.id = c.environment_id
WHERE e.project_id = sqlc.arg(project_id)::uuid AND e.id <> sqlc.arg(environment_id)::uuid
ORDER BY c.flag_id, e.position;
