-- name: CreateFeed :one
INSERT INTO feeds(
	id,
	title,
	description,
	url,
	created_at,
	updated_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	CURRENT_TIMESTAMP,
	CURRENT_TIMESTAMP
) 
RETURNING *;

-- name: UpdateFeed :one
UPDATE feeds SET last_fetched_at = $1 WHERE id = $2 RETURNING *;

-- name: FetchFeeds :many
SELECT 
	id,
	title,
	description
	url,
	last_fetched_at
FROM feeds
ORDER BY last_fetched_at DESC;
