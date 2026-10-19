-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: RotateRefreshToken :one
UPDATE refresh_tokens SET token_hash = sqlc.arg(new_hash),
    expires_at = sqlc.arg(expires_at), created_at = now()
WHERE token_hash = sqlc.arg(old_hash) AND expires_at > now()
RETURNING user_id;

-- name: DeleteRefreshToken :exec
DELETE FROM refresh_tokens WHERE token_hash = $1;
