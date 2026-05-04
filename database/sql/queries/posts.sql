-- name: CreatePost :one
INSERT INTO posts(
	id, created_at,	updated_at, title, url, description, published_at, feed_id
)
VALUES (@id, @now, @now, @title, @url, @description, @published_at, @feed_id)
RETURNING *;

-- name: GetPostsForUser :many
SELECT
    sqlc.embed(p),
	f.title as FeedTitle,
	f.url as FeedUrl,
	pu.is_read,
	pu.is_bookmarked,
    pu.is_archived
FROM posts as p

INNER JOIN feeds_users fu
ON p.feed_id = fu.feed_id AND fu.user_id = @user_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = @user_id

LEFT JOIN feeds f
ON fu.feed_id = f.id

WHERE fu.user_id = @user_id
AND (
    (published_at < COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP))
    OR (
        published_at = COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP)
        AND p.id < COALESCE(sqlc.narg(cursor_id), CAST('ffffffff-ffff-ffff-ffff-ffffffffffff' AS UUID))
    )
)
AND (pu.is_archived = False OR pu.is_archived IS NULL)
ORDER BY published_at DESC, p.id DESC
LIMIT 51;

-- name: GetArchivedPostsForUser :many
SELECT
    sqlc.embed(p),
	f.title as FeedTitle,
	f.url as FeedUrl,
	pu.is_read,
	pu.is_bookmarked,
    pu.is_archived
FROM posts as p

INNER JOIN feeds_users fu
ON p.feed_id = fu.feed_id AND fu.user_id = @user_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = @user_id

LEFT JOIN feeds f
ON fu.feed_id = f.id

WHERE fu.user_id = @user_id
AND (
    (published_at < COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP))
    OR (
        published_at = COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP)
        AND p.id < COALESCE(sqlc.narg(cursor_id), CAST('ffffffff-ffff-ffff-ffff-ffffffffffff' AS UUID))
    )
)
AND pu.is_archived = True
ORDER BY published_at DESC, p.id DESC
LIMIT 51;

-- name: GetBookmarkedPostsForUser :many
SELECT
    sqlc.embed(p),
	f.title as FeedTitle,
	f.url as FeedUrl,
	pu.is_read,
	pu.is_bookmarked,
    pu.is_archived
FROM posts as p

INNER JOIN feeds_users fu
ON p.feed_id = fu.feed_id AND fu.user_id = @user_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = @user_id

LEFT JOIN feeds f
ON fu.feed_id = f.id

WHERE fu.user_id = @user_id
AND (
    (published_at < COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP))
    OR (
        published_at = COALESCE(sqlc.narg(cursor_date), '9999-12-31'::TIMESTAMP)
        AND p.id < COALESCE(sqlc.narg(cursor_id), CAST('ffffffff-ffff-ffff-ffff-ffffffffffff' AS UUID))
    )
)
AND pu.is_bookmarked = True
ORDER BY published_at DESC, p.id DESC
LIMIT 51;

-- name: ToggleArchivedStatus :one
INSERT INTO posts_users(
    id,
    post_id,
    user_id,
    is_read,
    is_bookmarked,
    is_archived,
    created_at,
    updated_at
)
VALUES (@id, @post_id, @user_id, false, false, @is_archived, @now, @now)

ON CONFLICT (post_id, user_id) DO UPDATE SET
	is_archived = NOT posts_users.is_archived,
	updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: ToggleBookmarkStatus :one
INSERT INTO posts_users(
    id,
    post_id,
    user_id,
    is_read,
    is_bookmarked,
    is_archived,
    created_at,
    updated_at
)
VALUES (@id, @post_id, @user_id, false, @is_bookmarked, false, @now, @now)

ON CONFLICT (post_id, user_id) DO UPDATE SET
	is_bookmarked = NOT posts_users.is_bookmarked,
	updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: TogglePostReadStatus :one
INSERT INTO posts_users(
    id,
    post_id,
    user_id,
    is_read,
    is_bookmarked,
    is_archived,
    created_at,
    updated_at
)
VALUES (@id, @post_id, @user_id, @is_read, false, false, @now, @now)

ON CONFLICT (post_id, user_id) DO UPDATE SET
	is_read = NOT posts_users.is_read,
	updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: MarkPostAsRead :one
INSERT INTO posts_users(
    id,
    post_id,
    user_id,
    is_read,
    is_bookmarked,
    is_archived,
    created_at,
    updated_at
)
VALUES (@id, @post_id, @user_id, @is_read, false, false, @now, @now)

ON CONFLICT (post_id, user_id) DO UPDATE SET
	is_read = EXCLUDED.is_read,
	updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetPostByID :one
SELECT
	p.id,
	p.title,
	p.url,
	p.description,
	f.title as FeedTitle,
	f.url as FeedURL,
	pu.is_read,
    pu.is_bookmarked,
    pu.is_archived,
	p.published_at
FROM posts as p

INNER JOIN feeds_users fu
ON p.feed_id = fu.feed_id AND fu.user_id = @user_id

LEFT JOIN posts_users pu
ON p.id = pu.post_id AND pu.user_id = @user_id

LEFT JOIN feeds f
ON f.id = fu.feed_id

WHERE p.id = @post_id;
