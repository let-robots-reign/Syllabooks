-- name: CreateBook :one
INSERT INTO books (isbn, title, author, level, page_count)
VALUES (@isbn, @title, @author, @level, @page_count)
RETURNING *;
