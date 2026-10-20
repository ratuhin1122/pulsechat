-- name: ListUserRoomIDs :many
SELECT room_id FROM room_members WHERE room_members.user_id = $1 AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.room_id=room_members.room_id AND d.user_id=room_members.user_id AND d.hidden) ORDER BY room_id;
