-- Forum topic support for Telegram announcement targets.
-- The same group can be listed more than once with different topics.

-- +goose Up
ALTER TABLE telegram_chats
    ADD COLUMN message_thread_id BIGINT
        CHECK (message_thread_id IS NULL OR message_thread_id > 0);

ALTER TABLE telegram_chats
    DROP CONSTRAINT telegram_chats_edition_id_chat_id_key;

CREATE UNIQUE INDEX telegram_chats_edition_chat_thread_uidx
    ON telegram_chats (edition_id, chat_id, (COALESCE(message_thread_id, 0)));

-- +goose Down
DROP INDEX IF EXISTS telegram_chats_edition_chat_thread_uidx;

ALTER TABLE telegram_chats
    ADD CONSTRAINT telegram_chats_edition_id_chat_id_key UNIQUE (edition_id, chat_id);

ALTER TABLE telegram_chats
    DROP COLUMN IF EXISTS message_thread_id;
