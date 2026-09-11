Backend: Go 1.22+. Standard library net/http and ServeMux only.
  - NO web framework (no Gin, Echo, Fiber, Chi).
  - NO ORM (no GORM, Ent, Bun).
  - Postgres via pgx. Queries via sqlc, generated from plain SQL in db/queries/.
  - Migrations: plain SQL files, applied with goose.
  - Errors returned explicitly. No panic/recover as control flow.
Frontend: Vite + React + CSS Modules. No Tailwind, no component library.
Read ./specs/syllabooks-prd.md before starting. Section 5 lists settled decisions — do not re-litigate them.
This app serves ~25 users. Do not add caching, queues, background workers, interfaces with one implementation, or abstraction layers for scale.
