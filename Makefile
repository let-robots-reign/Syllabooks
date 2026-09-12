# Development tasks, run from the repo root. `db` needs the stack up
# (docker compose up --watch); `build` needs Go and pnpm on the host.

SCHEMA := backend/db/schema.sql
DIST := backend/cmd/api/dist

.PHONY: db build

# Apply pending migrations, write the resulting schema to backend/db/schema.sql
# for reading, and regenerate the sqlc code. Run after adding a migration or
# changing a query.
#
# Only the DDL is kept. pg_dump's SET lines and comment blocks are dropped for
# readability; its version lines and \restrict lines (a random key on every
# run) are dropped so schema.sql only changes when the schema does.
db:
	docker compose exec backend go tool goose up
	docker compose exec -T db sh -c 'pg_dump -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" --schema-only --no-owner --no-privileges --exclude-table=goose_db_version' \
		> $(SCHEMA).tmp || { rm -f $(SCHEMA).tmp; exit 1; }
	{ echo '-- Current schema, written by `make db`. Do not edit: add a migration.'; \
	  sed -E '/^\\(un)?restrict /d; /^SET /d; /^SELECT pg_catalog\.set_config/d; /^--( .*)?$$/d' $(SCHEMA).tmp | cat -s; } > $(SCHEMA)
	rm $(SCHEMA).tmp
	cd backend && go tool sqlc generate

# Build bin/syllabooks: the Go server with the built frontend embedded, one
# process serving the API and the app (PRD §12). The frontend bundle is copied
# into backend/cmd/api/dist, where go:embed can reach it.
build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build
	find $(DIST) -mindepth 1 ! -name .gitkeep -delete
	cp -R frontend/dist/. $(DIST)/
	cd backend && go build -o ../bin/syllabooks ./cmd/api
