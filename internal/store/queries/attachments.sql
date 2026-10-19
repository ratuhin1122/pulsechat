-- name: CreateAttachment :one
INSERT INTO attachments (
    message_id, kind, original_name, stored_name, mime_type, size_bytes,
    width, height, duration_sec, blurhash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE id = $1;

-- name: ListAttachmentsForMessage :many
SELECT * FROM attachments WHERE message_id = $1 ORDER BY created_at, id;

-- name: UpdateAttachment :one
UPDATE attachments SET mime_type = $2, size_bytes = $3, width = $4,
    height = $5, duration_sec = $6, blurhash = $7, status = $8
WHERE id = $1 RETURNING *;

-- name: DeleteAttachment :execrows
DELETE FROM attachments WHERE id = $1;

-- name: MarkAttachmentReady :execrows
UPDATE attachments SET status = 'ready', rejection_reason = NULL WHERE id = $1 AND status = 'pending';

-- name: RejectAttachment :execrows
UPDATE attachments SET status = 'rejected', rejection_reason = sqlc.arg(reason)::text
WHERE id = sqlc.arg(id) AND status <> 'rejected';
