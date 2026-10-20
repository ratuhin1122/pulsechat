ALTER TABLE room_members ADD COLUMN last_read_message_id uuid REFERENCES messages(id) ON DELETE SET NULL;
CREATE INDEX room_members_read_message_idx ON room_members (last_read_message_id) WHERE last_read_message_id IS NOT NULL;
