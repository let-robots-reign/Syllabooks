-- name: CreateBookRequest :exec
INSERT INTO book_requests (user_id, title)
VALUES (@user_id, @title);

-- name: ListBookRequests :many
-- Requests still to consider first, newest on top; bought ones sink below.
SELECT r.id,
       r.title,
       r.purchased_at,
       r.created_at,
       u.id AS student_id,
       u.display_name AS student_name
FROM book_requests r
JOIN users u ON u.id = r.user_id
ORDER BY (r.purchased_at IS NOT NULL), r.created_at DESC, r.id;

-- name: SetBookRequestPurchased :execrows
UPDATE book_requests
SET purchased_at = CASE WHEN sqlc.arg(purchased)::boolean THEN COALESCE(purchased_at, now()) END
WHERE id = @id;

-- name: DeleteBookRequest :execrows
DELETE FROM book_requests WHERE id = @id;
