-- name: CreatePendingUpload :one
WITH media_message AS (
    INSERT INTO messages (room_id, sender_id, type, body)
    SELECT sqlc.arg(room_id), sqlc.arg(sender_id), sqlc.arg(kind)::text, ''
    WHERE EXISTS (
        SELECT 1 FROM room_members
        WHERE room_id = sqlc.arg(room_id) AND user_id = sqlc.arg(sender_id)
    )
    RETURNING id
)
INSERT INTO attachments (
    message_id, kind, original_name, stored_name, mime_type, size_bytes, width, height, status
)
SELECT id, sqlc.arg(kind)::text, sqlc.arg(original_name), sqlc.arg(stored_name),
    sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.narg(width), sqlc.narg(height), 'pending'
FROM media_message
RETURNING *;
