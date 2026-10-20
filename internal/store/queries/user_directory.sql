-- name: SearchUsers :many
SELECT id, username, COALESCE(NULLIF(display_name, ''), username)::text AS display_name FROM users
WHERE id <> sqlc.arg(viewer_id)
  AND (starts_with(lower(username), lower(sqlc.arg(search)::text))
    OR starts_with(lower(display_name), lower(sqlc.arg(search)::text)))
ORDER BY lower(username), id LIMIT 20;

-- name: ListDirectRoomPeers :many
SELECT m.room_id, u.username FROM room_members m
JOIN users u ON u.id = m.user_id
JOIN rooms r ON r.id = m.room_id AND r.type = 'dm'
WHERE m.room_id = ANY(sqlc.arg(room_ids)::uuid[])
  AND m.user_id <> sqlc.arg(viewer_id);
