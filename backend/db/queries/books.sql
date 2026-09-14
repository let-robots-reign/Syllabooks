-- name: CreateBook :one
INSERT INTO books (isbn, title, author, level, page_count, description)
VALUES (@isbn, @title, @author, @level, @page_count, @description)
RETURNING *;

-- name: ListBooks :many
SELECT *
FROM books
ORDER BY lower(title), title, id;

-- name: GetBook :one
SELECT *
FROM books
WHERE id = @id;

-- name: UpdateBook :one
UPDATE books
SET isbn = @isbn,
    title = @title,
    author = @author,
    level = @level,
    page_count = @page_count,
    description = @description
WHERE id = @id
RETURNING *;

-- name: UpdateBookCover :one
UPDATE books
SET cover_url = @cover_url
WHERE id = @id
RETURNING *;

-- name: DeleteBook :one
DELETE FROM books
WHERE id = @id
RETURNING cover_url;
