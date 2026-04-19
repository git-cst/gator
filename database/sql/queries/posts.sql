-- name: CreatePost :one
INSERT INTO posts(
	id, created_at,	updated_at, title, url, description, published_at, feed_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPostsForUser :many
SELECT
	p.id,
	p.title,
	p.url,
	p.description,
	pu.is_read,
	p.published_at
FROM posts as p

LEFT JOIN feeds_users fu
ON p.feed_id = fu.feed_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = $1

WHERE fu.user_id = $1

ORDER BY p.published_at DESC
LIMIT 50
OFFSET $2;

-- name: TogglePostReadStatus :one
INSERT INTO posts_users(
	id,
	post_id,
	user_id,
	is_read,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6)

ON CONFLICT (post_id, user_id) DO UPDATE SET
	is_read = NOT posts_users.is_read,
	updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetPostByID :one
SELECT
	p.id,
	p.title,
	p.url,
	p.description,
	pu.is_read,
	p.published_at
FROM posts as p

LEFT JOIN feeds_users fu
ON p.feed_id = fu.feed_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = $1

WHERE p.id = $2;
