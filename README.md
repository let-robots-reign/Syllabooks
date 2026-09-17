# Syllabooks
Web app for running a book club in a state school

## Development

Needs Docker and Go 1.27 (Go runs sqlc and goose on the host), and pnpm for
`make build`.

```sh
cp .env.example .env
docker compose up --watch
```

The app is at http://localhost:8000. Caddy sends `/api/` to the Go backend and
everything else to the Vite dev server, so the browser sees one origin.

| Service    | What it runs                              | Reloads on change             |
| ---------- | ----------------------------------------- | ----------------------------- |
| `caddy`    | reverse proxy, port `HTTP_PORT`           | —                             |
| `frontend` | Vite dev server                           | HMR                           |
| `backend`  | `cmd/api`                                 | restarts under `--watch`      |
| `db`       | Postgres 18, port `POSTGRES_PORT` (local) | —                             |

### Configuration

All settings live in `.env` (git-ignored; `.env.example` is the template).
Compose reads it and hands the values to containers as environment variables.
The backend reads only its environment:

| Variable       | Default | Notes                                   |
| -------------- | ------- | --------------------------------------- |
| `DATABASE_URL` | —       | Required. Compose builds it from `POSTGRES_*`. |
| `LISTEN_ADDR`  | `:8080` |                                         |
| `PUBLIC_URL`   | —       | The address the browser uses, e.g. `https://syllabooks.ru`. OAuth redirect URIs are built from it; `https://` also makes cookies Secure. Compose sets `http://localhost:8000`. |
| `LOAN_DAYS`    | `21`    | Positive integer; borrowing period used to calculate the due date. |
| `SHELF_CODE`   | —       | Required outside Compose. Exact text encoded in the QR beside the shelf; Compose defaults to `SYLLABOOKS-LOCAL-SHELF` for development. |
| `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET` | — | Yandex OAuth app. Unset disables Yandex login. Redirect URI to register: `$PUBLIC_URL/api/auth/callback/yandex`. |
| `VK_CLIENT_ID` | — | VK ID app. Unset disables VK login. No secret: the code exchange uses PKCE. Trusted redirect URL: `$PUBLIC_URL/api/auth/callback/vk`. |

### Accounts for local testing

Code users are created by the teacher (admin UI comes later); until then, in
`docker compose exec db sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"'`:

```sql
INSERT INTO users (display_name, code) VALUES ('Тест Тестов', 'K7F2MX');
UPDATE users SET is_admin = true WHERE code = 'K7F2MX';  -- admin is set by hand (PRD §3)
```

Then sign in at http://localhost:8000 with the code; the first login asks you to
make up a password. Deleting a row from `sessions` signs that client out on its
next request.

To test returns locally, display or print a QR whose payload is exactly the
configured `SHELF_CODE`. The same value can be printed underneath for manual
entry; manual entry is deliberately logged as an unscanned return.

### Migrations (goose)

Plain SQL files in `backend/db/migrations/`.

```sh
cd backend && go tool goose -s -dir db/migrations create add_books sql   # new migration

make db   # from the repo root: apply, update schema.sql, regenerate sqlc code

docker compose exec backend go tool goose status
docker compose exec backend go tool goose down     # roll back one
```

Migrations are not applied automatically on startup.

`backend/db/schema.sql` is the whole current schema in one file: read it to see
what a table looks like instead of replaying migrations. `make db` rewrites it
from the database, so never edit it by hand. Keys, indexes and foreign keys come
after the tables as separate statements, so search for the table name.

### Queries (sqlc)

Write SQL in `backend/db/queries/`, then run `make db` (or just
`go tool sqlc generate` in `backend/`) to generate Go into `backend/db/gen/`.
sqlc reads the schema from the migrations, not from `schema.sql`.

### Tests

The loan-rule tests need a migrated database and roll back everything they
write:

```sh
docker compose exec backend go test ./...
```

### Frontend

Design tokens (palette, level colours, type scale, spacing) are CSS custom
properties in `frontend/src/tokens.scss`, taken from the Claude Design
screens. The primitives in `frontend/src/ui/` (`Button`, `TextField`, `Card`,
`Eyebrow`, `Wordmark`, `Link`) are built on them; screens use both rather than
new hex values. Fonts are bundled from `@fontsource`, not loaded from Google.

## Production build

```sh
make build
DATABASE_URL='postgres://syllabooks:syllabooks@localhost:5432/syllabooks?sslmode=disable' ./bin/syllabooks
```

`make build` builds the frontend, copies it into `backend/cmd/api/dist` and
compiles it into the binary with `go:embed`, so one file serves both the API
and the app (the example above borrows the compose database; the app is then
at http://localhost:8080). Paths under `/api/` go to the handlers; any other
path gets a file from the bundle or, failing that, `index.html`, so client-side
routes such as `/profile` survive a reload. In development
`backend/cmd/api/dist` holds only `.gitkeep`, and Caddy sends pages to Vite.

## CSV exports

An admin can download books, users, or loans from the `Выгрузить CSV` menu in
the teacher interface. The files are UTF-8 CSV with a byte-order mark so that
Cyrillic opens correctly in spreadsheet applications. They contain operational
library data, not authentication secrets. PostgreSQL backups are the recovery
source for the complete database.

## Database backups

[`ops/backup.sh`](ops/backup.sh) creates a PostgreSQL custom-format archive,
checks that `pg_restore` can read it, publishes it atomically, and retains the
newest 14 successful archives. It backs up PostgreSQL only. The files under
`BOOK_COVERS_DIR` need a separate filesystem backup if preserving locally
stored covers is required.

The production host needs `pg_dump` and `pg_restore` from a PostgreSQL client
version at least as new as the server. Install the script and configuration for
an unprivileged service account:

```sh
sudo install -d -m 700 -o syllabooks -g syllabooks /var/backups/syllabooks
sudo install -d -m 755 /etc/syllabooks /opt/syllabooks/ops
sudo install -m 755 ops/backup.sh /opt/syllabooks/ops/backup.sh
sudo install -m 600 -o syllabooks -g syllabooks ops/backup.env.example /etc/syllabooks/backup.env
sudoedit /etc/syllabooks/backup.env
```

Run one backup manually as that account before installing the cron entry:

```sh
sudo -u syllabooks sh -c '. /etc/syllabooks/backup.env && /opt/syllabooks/ops/backup.sh'
```

[`ops/crontab.example`](ops/crontab.example) runs it nightly at 03:00 Moscow
time. Install that line in the `syllabooks` account's crontab. Review
`/var/backups/syllabooks/backup.log` and periodically copy archives off the
VPS, for example:

```sh
scp 'syllabooks@server.example:/var/backups/syllabooks/syllabooks-*.dump' ./backups/
```

Keeping only copies on the application server does not protect against losing
that server.

### Restore test

Never test a restore against the production database. Create a new, clearly
named scratch database on the same PostgreSQL server and construct a URL that
names that scratch database explicitly:

```sh
export DATABASE_URL='postgres://syllabooks:replace-me@127.0.0.1:5432/syllabooks?sslmode=disable'
export SCRATCH_DB="syllabooks_restore_$(date -u +%Y%m%dT%H%M%SZ)"
export SCRATCH_DATABASE_URL="postgres://syllabooks:replace-me@127.0.0.1:5432/$SCRATCH_DB?sslmode=disable"
export BACKUP_FILE='/var/backups/syllabooks/syllabooks-YYYYMMDDTHHMMSSZ.dump'

test "$SCRATCH_DATABASE_URL" != "$DATABASE_URL"
case "$SCRATCH_DB" in syllabooks_restore_*) ;; *) exit 1 ;; esac
pg_restore --list "$BACKUP_FILE" >/dev/null
createdb --maintenance-db="$DATABASE_URL" "$SCRATCH_DB"
pg_restore --exit-on-error --no-owner --no-privileges --dbname="$SCRATCH_DATABASE_URL" "$BACKUP_FILE"
psql "$SCRATCH_DATABASE_URL" -v ON_ERROR_STOP=1 -c '\dt'
psql "$SCRATCH_DATABASE_URL" -v ON_ERROR_STOP=1 -c \
  'SELECT (SELECT count(*) FROM books) AS books, (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM loans) AS loans;'
```

Compare those three counts with the source database or with counts recorded at
backup time, then remove only the scratch database:

```sh
dropdb --maintenance-db="$DATABASE_URL" "$SCRATCH_DB"
```

If the restore or validation fails, keep the archive and investigate. Do not
prune or replace the most recent known-good off-server copy.
