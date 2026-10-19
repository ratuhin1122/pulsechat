-- name: ListUserRoomIDs :many
SELECT room_id FROM room_members WHERE user_id = $1 ORDER BY room_id;
