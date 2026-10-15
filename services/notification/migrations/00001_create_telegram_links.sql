-- +goose Up
-- Which Telegram chat receives a user's notifications. A user has at most one chat.
CREATE TABLE telegram_links (
    user_id   uuid PRIMARY KEY,
    chat_id   bigint      NOT NULL,
    linked_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX telegram_links_chat_id_idx ON telegram_links (chat_id);

-- +goose Down
DROP TABLE telegram_links;
