-- name: GetUsers :many
SELECT DISTINCT
	*
FROM users;

-- name: GetUserById :one
SELECT 
	*
FROM users
WHERE users.id = $1
LIMIT 1;

-- name: GetUserByUsername :one
SELECT
	*
FROM users
WHERE users.name = $1
LIMIT 1;
