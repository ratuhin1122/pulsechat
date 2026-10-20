DROP INDEX room_members_read_message_idx;
ALTER TABLE room_members DROP COLUMN last_read_message_id;
