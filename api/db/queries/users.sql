-- name: CreateUser :one
INSERT INTO users (email, name, role, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at;

-- name: UpdateUserRole :one
UPDATE users SET role = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetUserPassword :exec
UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1;

-- name: SetUserDisabled :one
UPDATE users
SET disabled_at = CASE WHEN sqlc.arg(disabled)::boolean THEN now() END, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CountActiveAdmins :one
SELECT count(*) FROM users WHERE role = 'admin' AND disabled_at IS NULL;

-- name: CreateUserToken :one
INSERT INTO user_tokens (user_id, purpose, token_hash, expires_at, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ConsumeUserToken :one
UPDATE user_tokens SET used_at = now()
WHERE token_hash = $1 AND purpose = $2 AND used_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: RevokeUserTokens :exec
UPDATE user_tokens SET used_at = now()
WHERE user_id = $1 AND purpose = $2 AND used_at IS NULL;
