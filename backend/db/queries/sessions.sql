-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at)
VALUES (@token_hash, @user_id, @expires_at);

-- name: GetSessionUser :one
-- The user behind a live session. Expired rows are ignored rather than
-- cleaned up: at this scale they cost nothing.
SELECT users.*
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = @token_hash AND sessions.expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = @token_hash;

-- name: DeleteUserSessions :exec
-- Signs a user out on every device. Required on ban and on password reset.
DELETE FROM sessions WHERE user_id = @user_id;
