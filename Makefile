# Development tasks, run from the repo root. `db` needs the stack up
# (docker compose up --watch); `build` needs Go and pnpm on the host.

SCHEMA := backend/db/schema.sql
DIST := backend/cmd/api/dist
LINUX_BINARY := bin/syllabooks-linux-amd64
DEPLOY_HOST ?=
DEPLOY_URL ?=

.PHONY: db frontend-build build build-linux deploy

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

# Build the frontend bundle where go:embed can reach it.
frontend-build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build
	find $(DIST) -mindepth 1 ! -name .gitkeep -delete
	cp -R frontend/dist/. $(DIST)/

# Build bin/syllabooks: one process serving the API and embedded app (PRD §12).
build: frontend-build
	cd backend && go build -o ../bin/syllabooks ./cmd/api

# Build on any Go host for the production linux/amd64 VPS. The server has no
# build toolchain or Node installation; it receives this one static binary.
build-linux: frontend-build
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../$(LINUX_BINARY) ./cmd/api

# Build, stream one binary through the restricted deployment command, run
# embedded migrations via ExecStartPre, restart, and verify the public HTTPS
# endpoint.
#
#   make deploy DEPLOY_HOST=zotov@31.77.173.53 DEPLOY_URL=https://syllabooks.ru
deploy:
	@test -n "$(DEPLOY_HOST)" || { echo 'DEPLOY_HOST is required (user@host)' >&2; exit 2; }
	@test -n "$(DEPLOY_URL)" || { echo 'DEPLOY_URL is required (https://domain)' >&2; exit 2; }
	$(MAKE) build-linux
	ssh "$(DEPLOY_HOST)" '/usr/local/sbin/syllabooks-deploy' < $(LINUX_BINARY)
	curl --fail --show-error --silent --retry 5 --retry-delay 2 --retry-connrefused "$(DEPLOY_URL)/api/health"
