CREATE TABLE personal_chat_deletions (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    room_id uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    cleared_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    hidden boolean NOT NULL DEFAULT true,
    PRIMARY KEY (user_id, room_id)
);

-- A durable cleanup outbox keeps failed disk deletions retryable after account removal.
CREATE TABLE storage_deletions (
    area text NOT NULL CHECK (area IN ('quarantine', 'clean')),
    name text NOT NULL,
    PRIMARY KEY (area, name)
);
