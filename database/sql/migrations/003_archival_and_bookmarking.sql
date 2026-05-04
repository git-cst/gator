-- +goose Up
-- +goose StatementBegin
ALTER TABLE posts_users
ADD COLUMN is_bookmarked BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose StatementEnd

-- +goose Down
ALTER TABLE posts_users
DROP COLUMN is_bookmarked,
DROP COLUMN is_archived;
