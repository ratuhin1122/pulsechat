ALTER TABLE attachments
    ADD COLUMN room_id uuid REFERENCES rooms(id) ON DELETE CASCADE,
    ADD COLUMN sender_id uuid REFERENCES users(id) ON DELETE CASCADE,
    ADD COLUMN notified_at timestamptz,
    ALTER COLUMN message_id DROP NOT NULL;

UPDATE attachments a SET room_id = m.room_id, sender_id = m.sender_id
FROM messages m WHERE a.message_id = m.id;

ALTER TABLE attachments ALTER COLUMN room_id SET NOT NULL, ALTER COLUMN sender_id SET NOT NULL;

-- Detach old pending/rejected placeholders before removing them from history.
DO $$
DECLARE unpublished_ids uuid[];
BEGIN
    SELECT array_agg(DISTINCT message_id) INTO unpublished_ids FROM attachments WHERE status <> 'ready';
    UPDATE attachments SET message_id = NULL WHERE status <> 'ready';
    DELETE FROM messages m WHERE m.id = ANY(unpublished_ids)
        AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.message_id = m.id);
END $$;

CREATE INDEX attachments_room_id_idx ON attachments (room_id);
CREATE INDEX attachments_sender_id_idx ON attachments (sender_id);
