-- Current schema, written by `make db`. Do not edit: add a migration.

CREATE TYPE public.book_level AS ENUM (
    'green',
    'yellow',
    'red'
);

CREATE TYPE public.return_reason AS ENUM (
    'finished',
    'too_hard',
    'boring',
    'skipped'
);

CREATE TYPE public.user_status AS ENUM (
    'approved',
    'pending',
    'banned'
);

CREATE TABLE public.books (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    isbn text,
    title text NOT NULL,
    author text NOT NULL,
    level public.book_level NOT NULL,
    page_count integer NOT NULL,
    cover_url text,
    is_lost boolean DEFAULT false NOT NULL,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    description text,
    CONSTRAINT books_description_length CHECK ((char_length(description) <= 240)),
    CONSTRAINT books_isbn_check CHECK ((isbn ~ '^[0-9]{13}$'::text)),
    CONSTRAINT books_page_count_check CHECK ((page_count > 0))
);

CREATE TABLE public.loans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    book_id uuid NOT NULL,
    user_id uuid NOT NULL,
    taken_at timestamp with time zone DEFAULT now() NOT NULL,
    due_at timestamp with time zone NOT NULL,
    returned_at timestamp with time zone,
    return_reason public.return_reason,
    shelf_scan_ok boolean,
    book_scan_ok boolean,
    created_by_admin boolean DEFAULT false NOT NULL,
    CONSTRAINT loans_book_scan_set_on_return CHECK (((returned_at IS NULL) = (book_scan_ok IS NULL))),
    CONSTRAINT loans_due_after_taken CHECK ((due_at > taken_at)),
    CONSTRAINT loans_reason_only_when_returned CHECK (((return_reason IS NULL) OR (returned_at IS NOT NULL))),
    CONSTRAINT loans_returned_after_taken CHECK ((returned_at >= taken_at)),
    CONSTRAINT loans_shelf_scan_set_on_return CHECK (((returned_at IS NULL) = (shelf_scan_ok IS NULL)))
);

CREATE TABLE public.sessions (
    token_hash text NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    oauth_provider text,
    oauth_subject text,
    display_name text NOT NULL,
    code text,
    password_hash text,
    email text,
    status public.user_status DEFAULT 'approved'::public.user_status NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT users_code_check CHECK ((code = upper(code))),
    CONSTRAINT users_oauth_identity_complete CHECK (((oauth_provider IS NULL) = (oauth_subject IS NULL))),
    CONSTRAINT users_oauth_or_code CHECK (((oauth_provider IS NULL) OR (code IS NULL))),
    CONSTRAINT users_oauth_provider_check CHECK ((oauth_provider = ANY (ARRAY['yandex'::text, 'vk'::text]))),
    CONSTRAINT users_password_only_with_code CHECK (((password_hash IS NULL) OR (code IS NOT NULL)))
);

ALTER TABLE ONLY public.books
    ADD CONSTRAINT books_isbn_key UNIQUE (isbn);

ALTER TABLE ONLY public.books
    ADD CONSTRAINT books_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.loans
    ADD CONSTRAINT loans_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (token_hash);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_code_key UNIQUE (code);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_oauth_provider_oauth_subject_key UNIQUE (oauth_provider, oauth_subject);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX loans_one_open_per_book ON public.loans USING btree (book_id) WHERE (returned_at IS NULL);

CREATE UNIQUE INDEX loans_one_open_per_user ON public.loans USING btree (user_id) WHERE (returned_at IS NULL);

ALTER TABLE ONLY public.loans
    ADD CONSTRAINT loans_book_id_fkey FOREIGN KEY (book_id) REFERENCES public.books(id);

ALTER TABLE ONLY public.loans
    ADD CONSTRAINT loans_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

