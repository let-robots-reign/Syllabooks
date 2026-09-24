-- +goose Up

-- Titles students would like the club to buy. Students only submit; the
-- teacher ticks a request once the book is bought, or deletes it. Requests
-- belong to their author and go when the account is deleted.
CREATE TABLE book_requests (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        text NOT NULL CHECK (btrim(title) <> '' AND char_length(title) <= 200),
    purchased_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE book_requests;
