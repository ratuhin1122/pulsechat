-- name: CreateMemberMessage :one
INSERT INTO messages (room_id, sender_id, type, body)
SELECT sqlc.arg(room_id), sqlc.arg(sender_id), 'text', sqlc.arg(body)
WHERE EXISTS (
    SELECT 1 FROM room_members
    WHERE room_id = sqlc.arg(room_id) AND user_id = sqlc.arg(sender_id)
)
RETURNING *;

-- name: ListRoomMemberIDs :many
SELECT user_id FROM room_members WHERE room_members.room_id = $1 AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.room_id=room_members.room_id AND d.user_id=room_members.user_id AND d.hidden) ORDER BY user_id;
