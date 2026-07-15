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

## Architecture

The code is organized into layers:

- `internal/budget` — pure domain logic (validation, aggregation, grouping,
  sorting, money-as-integer-cents, budgets, bills, months). No I/O.
- `internal/store` — SQLite data access (sqlc-generated queries plus a
  repository wrapper) and schema bootstrap.
- `internal/web` — `html/template` templates and the render layer.
- `internal/http` — routing and HTTP handlers (HTMX-aware).
- `cmd/server` — server bootstrap.
- `cmd/budgetctl` — admin CLI for seeding, inspecting, and clearing the database.

Core transactions are accessed through sqlc-generated queries; the newer
budgets, recurring rules, and bills tables use small hand-written SQL helpers in
the same `store` package. Schema and seed data live in `internal/store/*.sql`.

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

## Note on security

This is a single-user local application with **no authentication**. It is
intended to run locally (bound to `127.0.0.1`). Do not expose it to a public
network without adding access controls.
