-- name: AddRoomMember :one
INSERT INTO room_members (room_id, user_id) VALUES ($1, $2) RETURNING *;

-- name: GetRoomMember :one
SELECT * FROM room_members WHERE room_members.room_id = $1 AND room_members.user_id = $2
AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.room_id=room_members.room_id AND d.user_id=room_members.user_id AND d.hidden);

-- name: ListRoomMembers :many
SELECT * FROM room_members WHERE room_id = $1
ORDER BY joined_at, user_id LIMIT $2 OFFSET $3;

-- name: RemoveRoomMember :execrows
DELETE FROM room_members WHERE room_id = $1 AND user_id = $2;
