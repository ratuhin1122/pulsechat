-- name: GetRoomMessage :one
SELECT * FROM messages WHERE room_id = $1 AND id = $2;

-- name: ListRoomHistory :many
SELECT * FROM messages
WHERE messages.room_id = sqlc.arg(room_id)
    AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.user_id=sqlc.arg(user_id) AND d.room_id=messages.room_id AND (d.hidden OR (messages.created_at <= d.cleared_at OR EXISTS (SELECT 1 FROM attachments old_file WHERE old_file.message_id=messages.id AND old_file.created_at <= d.cleared_at))))
    AND (sqlc.narg(before_created_at)::timestamptz IS NULL
        OR (created_at, id) < (sqlc.narg(before_created_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetUserMessage :one
SELECT m.* FROM messages m
JOIN room_members member ON member.room_id = m.room_id
WHERE m.id = sqlc.arg(message_id) AND member.user_id = sqlc.arg(user_id);

-- name: GetLatestUserMessage :one
SELECT m.* FROM messages m
JOIN room_members member ON member.room_id = m.room_id
WHERE member.user_id = $1
    AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.user_id=member.user_id AND d.room_id=m.room_id AND (d.hidden OR (m.created_at <= d.cleared_at OR EXISTS (SELECT 1 FROM attachments old_file WHERE old_file.message_id=m.id AND old_file.created_at <= d.cleared_at))))
ORDER BY m.created_at DESC, m.id DESC LIMIT 1;

-- name: ListReplayMessages :many
SELECT m.* FROM messages m
JOIN room_members member ON member.room_id = m.room_id
WHERE member.user_id = sqlc.arg(user_id)
    AND NOT EXISTS (SELECT 1 FROM personal_chat_deletions d WHERE d.user_id=member.user_id AND d.room_id=m.room_id AND (d.hidden OR (m.created_at <= d.cleared_at OR EXISTS (SELECT 1 FROM attachments old_file WHERE old_file.message_id=m.id AND old_file.created_at <= d.cleared_at))))
    AND (m.created_at, m.id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
    AND (m.created_at, m.id) <= (sqlc.arg(through_created_at)::timestamptz, sqlc.arg(through_id)::uuid)
ORDER BY m.created_at, m.id
LIMIT sqlc.arg(page_limit);
