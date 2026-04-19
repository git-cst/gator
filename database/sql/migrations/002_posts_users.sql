-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS posts_users (
	id UUID PRIMARY KEY,
	post_id UUID NOT NULL,
	user_id UUID NOT NULL,
	is_read BOOLEAN NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,

	FOREIGN KEY (post_id) REFERENCES posts(id)
	ON DELETE CASCADE,

	FOREIGN KEY (user_id) REFERENCES users(id)
	ON DELETE CASCADE,

	UNIQUE (post_id, user_id)
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS posts_users;
