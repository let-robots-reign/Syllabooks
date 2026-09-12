# Syllabooks
Web app for running a book club in a state school

## Development

Needs Docker and Go 1.27 (Go runs sqlc and goose on the host).

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
| `YANDEX_CLIENT_ID`, `YANDEX_CLIENT_SECRET` | — | Yandex OAuth app. Unset disables Yandex login. Redirect URI to register: `$PUBLIC_URL/api/auth/yandex/callback`. |
| `VK_CLIENT_ID` | — | VK ID app. Unset disables VK login. No secret: the code exchange uses PKCE. Trusted redirect URL: `$PUBLIC_URL/api/auth/vk/callback`. |

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
