-- name: CreateRoom :one
INSERT INTO rooms (type, name, is_public) VALUES ($1, $2, $3) RETURNING *;

-- name: GetRoom :one
SELECT * FROM rooms WHERE id = $1;

-- name: ListRoomsForUser :many
SELECT r.* FROM rooms r
JOIN room_members m ON m.room_id = r.id
WHERE m.user_id = $1 ORDER BY r.created_at, r.id LIMIT $2 OFFSET $3;

-- name: UpdateRoom :one
UPDATE rooms SET name = $2 WHERE id = $1 RETURNING *;

-- name: DeleteRoom :execrows
DELETE FROM rooms WHERE id = $1;
