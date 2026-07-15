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

## Note on security

This is a single-user local application with **no authentication**. It is
intended to run locally (bound to `127.0.0.1`). Do not expose it to a public
network without adding access controls.
