-- +goose Up

-- Levels describe the book, not the reader (PRD §6).
CREATE TYPE book_level AS ENUM ('green', 'yellow', 'red');
CREATE TYPE user_status AS ENUM ('approved', 'pending', 'banned');
CREATE TYPE return_reason AS ENUM ('finished', 'too_hard', 'boring', 'skipped');

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    oauth_provider text CHECK (oauth_provider IN ('yandex', 'vk')),
    oauth_subject  text,
    -- Teacher-editable: OAuth names are whatever is on the student's profile.
    display_name   text NOT NULL,
    -- Permanent username for code login. Stored uppercase; login uppercases
    -- the input, which makes it case-insensitive.
    code           text UNIQUE CHECK (code = upper(code)),
    -- Argon2id. NULL for OAuth users, and for a code user who has not set a
    -- password yet or whose password the teacher has reset.
    password_hash  text,
    email          text,
    status         user_status NOT NULL DEFAULT 'approved',
    is_admin       boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),

    UNIQUE (oauth_provider, oauth_subject),
    CONSTRAINT users_oauth_identity_complete CHECK ((oauth_provider IS NULL) = (oauth_subject IS NULL)),
    -- The two auth paths are disjoint: an OAuth identity or a code, never both.
    CONSTRAINT users_oauth_or_code CHECK (oauth_provider IS NULL OR code IS NULL),
    CONSTRAINT users_password_only_with_code CHECK (password_hash IS NULL OR code IS NOT NULL)
);

-- No availability column: a book is out exactly when it has an open loan.
CREATE TABLE books (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- The 13 digits under the back-cover barcode: the scan target and the only
    -- thing a student types. NULL for a book without one, which can only be
    -- borrowed through the teacher. Unique so a scan finds exactly one book.
    isbn       text UNIQUE CHECK (isbn ~ '^[0-9]{13}$'),
    title      text NOT NULL,
    author     text NOT NULL,
    level      book_level NOT NULL,
    page_count integer NOT NULL CHECK (page_count > 0),
    -- Locally stored cover image. NULL until the cover is sourced.
    cover_url  text,
    is_lost    boolean NOT NULL DEFAULT false,
    -- Teacher's private notes.
    notes      text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE loans (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- No ON DELETE CASCADE: finished loans are the class counter, so deleting
    -- a book or user with history must be a deliberate step.
    book_id          uuid NOT NULL REFERENCES books (id),
    user_id          uuid NOT NULL REFERENCES users (id),
    taken_at         timestamptz NOT NULL DEFAULT now(),
    due_at           timestamptz NOT NULL,
    -- NULL while the book is out.
    returned_at      timestamptz,
    return_reason    return_reason,
    -- Set on return, NULL while the book is out. Two flags rather than one:
    -- "scanned the shelf but not the book" and the reverse are different
    -- stories at the weekly shelf check.
    shelf_scan_ok    boolean,
    book_scan_ok     boolean,
    -- The teacher borrowed or returned on the student's behalf.
    created_by_admin boolean NOT NULL DEFAULT false,

    CONSTRAINT loans_due_after_taken CHECK (due_at > taken_at),
    CONSTRAINT loans_returned_after_taken CHECK (returned_at >= taken_at),
    CONSTRAINT loans_reason_only_when_returned CHECK (return_reason IS NULL OR returned_at IS NOT NULL),
    CONSTRAINT loans_shelf_scan_set_on_return CHECK ((returned_at IS NULL) = (shelf_scan_ok IS NULL)),
    CONSTRAINT loans_book_scan_set_on_return CHECK ((returned_at IS NULL) = (book_scan_ok IS NULL))
);

-- The loan rules. With no ORM these indexes are the only thing stopping two
-- students taking the same book at once; a violation (SQLSTATE 23505, with
-- the index name as the constraint) must reach the client as a 409, not a 500.
CREATE UNIQUE INDEX loans_one_open_per_book ON loans (book_id) WHERE returned_at IS NULL;
CREATE UNIQUE INDEX loans_one_open_per_user ON loans (user_id) WHERE returned_at IS NULL;

CREATE TABLE sessions (
    -- SHA-256 of the opaque token. The token itself is never stored.
    token_hash text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

-- +goose Down

DROP TABLE sessions;
DROP TABLE loans;
DROP TABLE books;
DROP TABLE users;
DROP TYPE return_reason;
DROP TYPE user_status;
DROP TYPE book_level;
