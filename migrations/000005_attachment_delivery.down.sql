WITH restored AS (
    INSERT INTO messages (id, room_id, sender_id, type, body, created_at)
    SELECT id, room_id, sender_id, kind, '', created_at FROM attachments WHERE message_id IS NULL
    RETURNING id
)
UPDATE attachments a SET message_id = restored.id FROM restored WHERE a.id = restored.id AND a.message_id IS NULL;

ALTER TABLE attachments ALTER COLUMN message_id SET NOT NULL,
    DROP COLUMN notified_at, DROP COLUMN room_id, DROP COLUMN sender_id;
