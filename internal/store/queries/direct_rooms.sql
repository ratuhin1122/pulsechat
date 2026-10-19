-- name: LockDirectRoomPair :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(pair_key)::text, 0));

-- name: FindDirectRoom :one
SELECT r.* FROM rooms r
JOIN room_members first_member ON first_member.room_id = r.id
JOIN room_members second_member ON second_member.room_id = r.id
WHERE r.type = 'dm'
    AND first_member.user_id = sqlc.arg(first_user)
    AND second_member.user_id = sqlc.arg(second_user)
    AND (SELECT count(*) FROM room_members WHERE room_id = r.id) = 2
ORDER BY r.created_at, r.id
LIMIT 1;

-- name: JoinGroupRoom :execrows
INSERT INTO room_members (room_id, user_id)
SELECT id, sqlc.arg(user_id) FROM rooms
WHERE id = sqlc.arg(room_id) AND type = 'group'
ON CONFLICT (room_id, user_id) DO NOTHING;
