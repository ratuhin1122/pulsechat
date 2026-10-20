DROP INDEX rooms_public_created_at_idx;
ALTER TABLE rooms DROP CONSTRAINT rooms_public_groups_only;
ALTER TABLE rooms DROP COLUMN is_public;
ALTER TABLE users DROP COLUMN display_name;
