-- name: UpsertTelegramLink :exec
-- Linking again (e.g. from another phone) replaces the previous chat.
INSERT INTO telegram_links (user_id, chat_id)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET chat_id = EXCLUDED.chat_id, linked_at = now();

-- name: GetTelegramLink :one
SELECT * FROM telegram_links
WHERE user_id = $1;

-- name: DeleteTelegramLink :exec
DELETE FROM telegram_links
WHERE user_id = $1;

-- name: DeleteTelegramLinksByChat :execrows
-- Used when the user sends /stop to the bot or blocks it.
DELETE FROM telegram_links
WHERE chat_id = $1;
