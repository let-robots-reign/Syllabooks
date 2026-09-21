-- +goose Up

CREATE SEQUENCE books_ord_seq;
ALTER TABLE books ADD COLUMN ord integer NOT NULL DEFAULT nextval('books_ord_seq');
ALTER SEQUENCE books_ord_seq OWNED BY books.ord;

WITH ordered AS (
    SELECT id, row_number() OVER (ORDER BY lower(title), title, id) AS ord
    FROM books
)
UPDATE books
SET ord = ordered.ord
FROM ordered
WHERE books.id = ordered.id;

SELECT setval(
    'books_ord_seq',
    COALESCE((SELECT max(ord) FROM books), 1),
    EXISTS (SELECT 1 FROM books)
);

-- +goose Down

ALTER TABLE books DROP COLUMN ord;
