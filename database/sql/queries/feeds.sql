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
	f.id,
	f.title,
	f.description,
	f.url,
	fu.user_id
FROM feeds f
LEFT JOIN feeds_users fu
ON fu.feed_id = f.id AND fu.user_id = $1

WHERE fu.user_id = $1
ORDER BY f.title ASC;


-- name: GetFeedByUrl :one
SELECT
	id,
	title,
	description,
	url,
	last_fetched_at
FROM feeds
WHERE url = $1
LIMIT 1;

-- name: GetFeedByID :one
SELECT
	id,
	title,
	description,
	url,
	last_fetched_at
FROM feeds
WHERE id = $1
LIMIT 1;

-- name: GetUserFeeds :many
SELECT
	f.id,
	f.title,
	f.description,
	f.url
FROM feeds_users fu

LEFT JOIN feeds f
	ON fu.feed_id = f.id
LEFT JOIN users u
	ON fu.user_id = u.id

WHERE u.id = $1
AND f.id < COALESCE(sqlc.narg(cursor), 'ffffffff-ffff-ffff-ffff-ffffffffffff')
ORDER BY f.id DESC
LIMIT 51;

-- name: AddFeedForUser :one
WITH inserted AS (
	INSERT INTO feeds_users(
		id,
		feed_id,
		user_id,
		created_at,
		updated_at
	) VALUES ($1, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	RETURNING *
)
SELECT inserted.feed_id, inserted.user_id, f.title, f.url, f.description
FROM inserted
JOIN feeds AS f ON f.id = inserted.feed_id;

-- name: DeleteFeedForUser :one
WITH deleted AS (
    DELETE FROM feeds_users
    WHERE user_id = $1 AND feed_id=$2
    RETURNING feed_id, user_id
)
SELECT deleted.feed_id, deleted.user_id, f.title, f.url, f.description
FROM deleted
JOIN feeds AS f ON f.id = deleted.feed_id;

-- name: DeleteFeed :one
DELETE FROM feeds WHERE id = $1
RETURNING *;
