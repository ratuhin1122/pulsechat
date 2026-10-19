-- name: CreateMemberMessage :one
INSERT INTO messages (room_id, sender_id, type, body)
SELECT sqlc.arg(room_id), sqlc.arg(sender_id), 'text', sqlc.arg(body)
WHERE EXISTS (
    SELECT 1 FROM room_members
    WHERE room_id = sqlc.arg(room_id) AND user_id = sqlc.arg(sender_id)
)
RETURNING *;

-- name: ListRoomMemberIDs :many
SELECT user_id FROM room_members WHERE room_id = $1 ORDER BY user_id;
