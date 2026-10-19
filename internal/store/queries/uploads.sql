-- name: CreatePendingUpload :one
INSERT INTO attachments (
    room_id, sender_id, kind, original_name, stored_name, mime_type, size_bytes, width, height, status
)
SELECT sqlc.arg(room_id), sqlc.arg(sender_id), sqlc.arg(kind)::text, sqlc.arg(original_name), sqlc.arg(stored_name),
    sqlc.arg(mime_type), sqlc.arg(size_bytes), sqlc.narg(width), sqlc.narg(height), 'pending'
WHERE EXISTS (
    SELECT 1 FROM room_members WHERE room_id = sqlc.arg(room_id) AND user_id = sqlc.arg(sender_id)
)
RETURNING *;
