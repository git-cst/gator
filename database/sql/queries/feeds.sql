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
	description,
	url,
	last_fetched_at
FROM feeds
ORDER BY last_fetched_at DESC;

-- name: GetDistinctFeeds :many
SELECT DISTINCT
	id,
	title,
	description,
	url,
	last_fetched_at
FROM feeds
ORDER BY title DESC;

-- name: GetDistinctFeedsForUser :many
SELECT DISTINCT
	f.title,
	f.description,
	f.url
FROM feeds_users fu
LEFT JOIN feeds f
ON fu.feed_id = f.id
LEFT JOIN users u
ON u .user_id = u.id

WHERE u.id = $1
ORDER BY title DESC;

-- name: FetchFeedByUrl :one
SELECT
	id,
	title,
	description,
	url,
	last_fetched_at
FROM feeds
WHERE url = $1
LIMIT 1;

-- name: GetUserFeeds :many
SELECT
	f.title,
	f.description,
	f.url
FROM feeds_users fu

LEFT JOIN feeds f
	ON fu.feed_id = f.id
LEFT JOIN users u
	ON fu.user_id = u.id

WHERE u.id = $1;
