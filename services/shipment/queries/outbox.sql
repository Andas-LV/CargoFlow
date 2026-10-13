-- name: InsertOutbox :exec
INSERT INTO outbox (id, subject, payload, headers)
VALUES ($1, $2, $3, $4);

-- name: LockUnsentOutbox :many
-- SKIP LOCKED lets several relay instances work in parallel without double-sending.
SELECT id, subject, payload, headers
FROM outbox
WHERE sent_at IS NULL
ORDER BY created_at
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxSent :exec
UPDATE outbox
SET sent_at = now()
WHERE id = ANY(@ids::uuid[]);
