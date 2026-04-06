-- name: CreatePost :one
INSERT INTO posts(
	created_at,
	updated_at,
	title,
	url,
	description,
	published_at,
	feed_id
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
RETURNING *;

-- name: GetPostsForUser :many
SELECT
	p.id,
	p.title,
	p.url,
	p.description,
	p.published_at
FROM posts as p

LEFT JOIN feeds_users fu
ON p.feed_id = fu.feed_id

WHERE fu.user_id = $1

ORDER BY p.published_at DESC
OFFSET $2;

