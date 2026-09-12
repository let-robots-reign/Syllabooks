-- name: CreateCodeUser :one
-- A code-based student, created by the teacher before their first login. The
-- password stays NULL until the student sets one.
INSERT INTO users (display_name, code)
VALUES (@display_name, sqlc.arg(code)::text)
RETURNING *;

-- name: GetUserByCode :one
-- The caller uppercases the code, which makes login case-insensitive.
SELECT * FROM users WHERE code = sqlc.arg(code)::text;

-- name: SetPasswordIfUnset :execrows
-- First login of a code user, or the first after a reset. The IS NULL check
-- stops two devices racing to set a password from overwriting each other:
-- the loser updates 0 rows.
UPDATE users SET password_hash = sqlc.arg(password_hash)::text
WHERE id = @id AND password_hash IS NULL;

-- name: ResetPassword :execrows
-- The teacher's reset. The student's next code entry lands in the "set a
-- password" branch. Only code users have a password, so 0 rows means there
-- is no such code user.
UPDATE users SET password_hash = NULL
WHERE id = @id AND code IS NOT NULL;

-- name: UpsertOAuthUser :one
-- Finds or creates the user for an OAuth identity. A repeat login changes
-- nothing, because display_name belongs to the teacher once the row exists;
-- the no-op SET is only there so RETURNING yields the existing row.
INSERT INTO users (oauth_provider, oauth_subject, display_name)
VALUES (sqlc.arg(oauth_provider)::text, sqlc.arg(oauth_subject)::text, @display_name)
ON CONFLICT (oauth_provider, oauth_subject) DO UPDATE SET oauth_provider = excluded.oauth_provider
RETURNING *;
