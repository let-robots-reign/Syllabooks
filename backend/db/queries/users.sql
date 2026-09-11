-- name: CreateCodeUser :one
-- A code-based student, created by the teacher before their first login. The
-- password stays NULL until the student sets one.
INSERT INTO users (display_name, code)
VALUES (@display_name, sqlc.arg(code)::text)
RETURNING *;
