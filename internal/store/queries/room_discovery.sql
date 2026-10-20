-- name: ListPublicRooms :many
SELECT * FROM rooms
WHERE type = 'group' AND is_public
  AND (sqlc.arg(search)::text = '' OR strpos(lower(name), lower(sqlc.arg(search)::text)) > 0)
ORDER BY created_at, id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListRoomSummaries :many
SELECT member.room_id,
  (SELECT count(*) FROM messages unread
   WHERE unread.room_id = member.room_id AND unread.sender_id <> member.user_id
     AND (cursor.id IS NULL OR (unread.created_at, unread.id) > (cursor.created_at, cursor.id)))::bigint AS unread_count,
  latest.id AS last_message_id, latest.sender_id AS last_sender_id,
  COALESCE(latest.type, '')::text AS last_type, COALESCE(left(latest.body, 280), '')::text AS last_body,
  latest.created_at AS last_created_at
FROM room_members member
LEFT JOIN messages cursor ON cursor.id = member.last_read_message_id AND cursor.room_id = member.room_id
LEFT JOIN LATERAL (
  SELECT m.* FROM messages m WHERE m.room_id = member.room_id
  ORDER BY m.created_at DESC, m.id DESC LIMIT 1
) latest ON true
WHERE member.user_id = sqlc.arg(user_id) AND member.room_id = ANY(sqlc.arg(room_ids)::uuid[]);

-- name: LockRoomMember :one
SELECT * FROM room_members WHERE room_id = $1 AND user_id = $2 FOR UPDATE;

-- name: SetRoomReadMarker :exec
UPDATE room_members SET last_read_message_id = sqlc.arg(message_id)
WHERE room_id = sqlc.arg(room_id) AND user_id = sqlc.arg(user_id);
