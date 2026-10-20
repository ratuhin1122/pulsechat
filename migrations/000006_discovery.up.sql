ALTER TABLE users ADD COLUMN display_name text NOT NULL DEFAULT '';
UPDATE users SET display_name = username;
ALTER TABLE rooms ADD COLUMN is_public boolean NOT NULL DEFAULT false;
ALTER TABLE rooms ADD CONSTRAINT rooms_public_groups_only CHECK (NOT is_public OR type = 'group');
CREATE INDEX rooms_public_created_at_idx ON rooms (created_at, id) WHERE is_public AND type = 'group';
