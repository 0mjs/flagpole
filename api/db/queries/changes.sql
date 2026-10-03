-- name: CreateChangeRequest :one
INSERT INTO change_requests (flag_id, environment_id, author_id, base_version, proposed, comment)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetChangeRequest :one
SELECT cr.*, f.key AS flag_key, e.key AS environment_key, a.name AS author_name, r.name AS reviewer_name
FROM change_requests cr
JOIN flags f ON f.id = cr.flag_id
JOIN environments e ON e.id = cr.environment_id
JOIN users a ON a.id = cr.author_id
LEFT JOIN users r ON r.id = cr.reviewer_id
WHERE cr.id = $1 AND f.project_id = $2;

-- name: ListChangeRequests :many
SELECT cr.*, f.key AS flag_key, e.key AS environment_key, a.name AS author_name, r.name AS reviewer_name
FROM change_requests cr
JOIN flags f ON f.id = cr.flag_id
JOIN environments e ON e.id = cr.environment_id
JOIN users a ON a.id = cr.author_id
LEFT JOIN users r ON r.id = cr.reviewer_id
WHERE f.project_id = $1
  AND (sqlc.narg(status)::change_status IS NULL OR cr.status = sqlc.narg(status))
ORDER BY cr.created_at DESC
LIMIT 200;

-- name: ReviewChangeRequest :one
UPDATE change_requests
SET status = $2, reviewer_id = $3, review_comment = $4, reviewed_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;
