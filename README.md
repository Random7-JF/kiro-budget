# kiro-budget

A single-user personal budget tracker built in Go. It serves a server-rendered
HTML interface enhanced with [HTMX](https://htmx.org/) for partial page updates,
and persists data in SQLite.

## Features

- Record income and expense transactions (add / edit / delete) with categories
  and notes, using an inline HTMX form with category autocomplete.
- Month-scoped dashboard with income, expense, and net-balance totals, plus a
  spending-by-category chart. Navigate between months.
- Per-category monthly **budgets** with progress bars and over-budget warnings.
- **Recurring** transaction rules that can be posted for a month (idempotent).
- **Bills**: track payees, amounts, and frequency (weekly, bi-weekly, monthly,
  quarterly, yearly) with a breakdown normalized to a weekly / monthly / yearly
  view.
- Filter and sort the transaction log; group by day, week, or month.
- Selectable UI themes (Light, Dark, Ocean, Sunset).
- **User accounts** with bcrypt-hashed passwords and cookie sessions; each
  user's transactions, budgets, recurring rules, and bills are fully isolated.
  A demo account (`test` / `password123`) is created automatically for local use.

## Architecture

The code is organized into layers:

- `internal/budget` — pure domain logic (validation, aggregation, grouping,
  sorting, money-as-integer-cents, budgets, bills, months). No I/O.
- `internal/store` — SQLite data access (hand-written, parameterized SQL in a
  repository), users/sessions, schema bootstrap, and migrations.
- `internal/auth` — password hashing (bcrypt), session tokens, demo-user setup.
- `internal/web` — `html/template` templates and the render layer.
- `internal/http` — routing, auth middleware, and HTTP handlers (HTMX-aware).
- `cmd/server` — server bootstrap.
- `cmd/budgetctl` — admin CLI for seeding, inspecting, and clearing the database.

All data access uses hand-written parameterized SQL scoped by `user_id`. Schema
and seed data live in `internal/store/*.sql`.

### Project layout

```
cmd/
  server/       HTTP server entry point
  budgetctl/    Admin CLI (seed / reset / dump / stats)
internal/
  budget/       Pure domain logic (money, validation, aggregation, budgets, bills, months)
  store/        SQLite data access, users/sessions, schema (schema.sql), seed data (seed.sql)
  auth/         Password hashing, session tokens, demo-user bootstrap
  web/          html/template templates and render layer
  http/         Routing, auth middleware, and HTMX-aware handlers
```

Money is stored and computed as integer cents to avoid floating-point rounding
errors. Correctness of the domain layer is covered by property-based tests
(`pgregory.net/rapid`) alongside example-based and integration tests.

## Running

Requires Go 1.25+.

```sh
go run ./cmd/server -db budget.db -addr 127.0.0.1:8080
```

Then open http://127.0.0.1:8080 in your browser.

Flags (also configurable via environment variables):

| Flag    | Env          | Default      | Description                    |
| ------- | ------------ | ------------ | ------------------------------ |
| `-db`   | `BUDGET_DB`  | `budget.db`  | Path to the SQLite database    |
| `-addr` | `BUDGET_ADDR`| `:8080`      | HTTP listen address            |

The schema is created automatically on first run.

## Testing

```sh
go test ./...
```

## Development & testing tooling

`budgetctl` is a small admin CLI for the SQLite database, useful for seeding a
known dataset, inspecting the data, or clearing it between test runs.

| Command  | Description                                                        |
| -------- | ------------------------------------------------------------------ |
| `seed`   | Replace all data with the built-in demo dataset, then print stats  |
| `stats`  | Print row counts and overall income / expense / net totals         |
| `dump`   | Print every row (transactions, budgets, recurring rules, bills)    |
| `reset`  | Delete all data (schema preserved); requires `-force`              |

```sh
# Replace all data with the built-in demo dataset (internal/store/seed.sql)
go run ./cmd/budgetctl seed -db budget.db

# Print row counts and overall income/expense/net totals
go run ./cmd/budgetctl stats -db budget.db

# Print every row (transactions, budgets, recurring rules, bills)
go run ./cmd/budgetctl dump -db budget.db

# Delete all data (schema preserved); -force is required
go run ./cmd/budgetctl reset -db budget.db -force
```

The `-db` flag defaults to `budget.db` (or `$BUDGET_DB`). The seed dataset lives
in [`internal/store/seed.sql`](internal/store/seed.sql) and is embedded into the
binary; `seed` clears existing data first, so it is idempotent.

## Code style

- Formatted with `gofmt`; run `gofmt -l internal cmd` to check.
- Line endings are normalized to LF via `.gitattributes` for consistency across
  platforms.
- The demo dataset in `internal/store/seed.sql` uses integer cents and covers
  May–July 2026 for one person (transactions, budgets, recurring rules, bills).

## Accounts & sessions

Passwords are hashed with bcrypt and sessions are tracked server-side with an
HttpOnly cookie. On startup the server ensures a demo account exists:

- Username: `test`
- Password: `password123`

Sign in at `/login`. Each account's financial data is isolated by `user_id`.
An older single-user database is migrated automatically on first startup: the
`user_id` columns are added and existing rows are assigned to the demo user.

## Note on security

There is no self-service registration UI, and it is intended to run locally
(bound to `127.0.0.1`). Sessions are not currently marked `Secure` (no TLS in
the local setup). Do not expose it to a public network without adding TLS and
hardening account management.
