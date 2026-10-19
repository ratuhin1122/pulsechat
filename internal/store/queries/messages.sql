-- name: CreateMessage :one
INSERT INTO messages (room_id, sender_id, type, body)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetMessage :one
SELECT * FROM messages WHERE id = $1;

-- name: ListMessages :many
SELECT * FROM messages WHERE room_id = $1
ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3;

-- name: UpdateMessage :one
UPDATE messages SET body = $2 WHERE id = $1 RETURNING *;

-- name: DeleteMessage :execrows
DELETE FROM messages WHERE id = $1;
