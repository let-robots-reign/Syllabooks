# Development tasks. Run from the repo root with the stack up
# (docker compose up --watch).

SCHEMA := backend/db/schema.sql

.PHONY: db

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
