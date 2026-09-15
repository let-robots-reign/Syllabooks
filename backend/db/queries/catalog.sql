-- name: ListCatalogBooks :many
SELECT b.id,
       b.isbn,
       b.title,
       b.author,
       b.level,
       b.page_count,
       b.description,
       b.cover_url,
       u.display_name AS borrower_name,
       l.due_at
FROM books b
LEFT JOIN loans l ON l.book_id = b.id AND l.returned_at IS NULL
LEFT JOIN users u ON u.id = l.user_id
WHERE NOT b.is_lost
ORDER BY lower(b.title), b.title, b.id;

-- name: GetCatalogBook :one
SELECT b.id,
       b.isbn,
       b.title,
       b.author,
       b.level,
       b.page_count,
       b.description,
       b.cover_url,
       u.display_name AS borrower_name,
       l.due_at
FROM books b
LEFT JOIN loans l ON l.book_id = b.id AND l.returned_at IS NULL
LEFT JOIN users u ON u.id = l.user_id
WHERE b.id = @id AND NOT b.is_lost;

-- name: CountFinishedBooks :one
SELECT count(*)
FROM loans
WHERE return_reason = 'finished';
