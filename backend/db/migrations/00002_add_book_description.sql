-- +goose Up

ALTER TABLE books
    ADD COLUMN description text,
    ADD CONSTRAINT books_description_length CHECK (char_length(description) <= 240);

-- +goose Down

ALTER TABLE books
    DROP CONSTRAINT books_description_length,
    DROP COLUMN description;
