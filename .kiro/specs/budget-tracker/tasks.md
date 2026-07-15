# Implementation Plan: Budget Tracker

## Overview

This plan implements the Budget Tracker in Go following the layered design: a pure domain layer (money-as-integer-cents, validation, aggregation, grouping, sorting, query-param parsing), a sqlc/SQLite data access layer with schema bootstrap, an `html/template` presentation layer, and HTMX-driven HTTP handlers. Work proceeds bottom-up: the pure domain logic is built and property-tested first, then persistence, then templates, then handlers wire everything together into full-page and fragment responses.

Property-based tests use `pgregory.net/rapid` (minimum 100 iterations each) and are tagged with a comment in the format `// Feature: budget-tracker, Property {number}: {property_text}`. Each of the 20 correctness properties maps to its own optional property-test sub-task placed next to the code it validates.

## Tasks

- [x] 1. Set up project structure, dependencies, and data models
  - [x] 1.1 Initialize Go module and directory layout
    - Create the Go module and the package directories `internal/budget`, `internal/store`, `internal/http`, `internal/web/templates`, and `cmd/server`
    - Add dependencies: SQLite driver, `pgregory.net/rapid`, and configure sqlc (`sqlc.yaml`) with the query/schema source paths
    - Verify the module builds with an empty `go build ./...`
    - _Requirements: 10.1_

  - [x] 1.2 Define domain types
    - Add `TxType`, `TimePeriod`, `SortField`, `SortDir`, `TypeFilter` type definitions and their allowed constant values
    - Add `Transaction`, `TransactionInput`, `FieldError`, `CategoryExpense`, `DashboardSummary`, and `Group` structs in `internal/budget`
    - _Requirements: 1.1, 2.1, 6.1_

- [x] 2. Implement money handling (integer cents)
  - [x] 2.1 Implement ParseAmount and FormatAmount
    - Write `ParseAmount(text string) (cents int64, err error)` rejecting missing, non-numeric, ≤ 0, > 999,999,999.99, and more-than-two-decimal-place inputs; accepting values in 0.01–999,999,999.99
    - Write `FormatAmount(cents int64) string` producing a fixed two-decimal string
    - _Requirements: 1.1, 1.2, 2.1, 2.2, 5.1, 6.1_

  - [x] 2.2 Write property test for amount validation
    - **Property 2: Amount validation rejects all invalid amounts** (accepts valid 0.01–999,999,999.99 with ≤ 2 decimals; rejects missing/non-numeric/≤0/>max/>2-decimals with an Amount field error)
    - **Validates: Requirements 1.2, 2.2, 3.3**

  - [x] 2.3 Write property test for amount formatting
    - **Property 10: Amount formatting always has exactly two decimals** (for any non-negative cents, output has exactly two decimal digits representing the same value)
    - **Validates: Requirements 5.1, 6.1**

- [x] 3. Implement transaction validation
  - [x] 3.1 Implement Validate for create and edit input
    - Write `Validate(input TransactionInput) (Transaction, []FieldError)` covering amount (via ParseAmount), ISO 8601 `YYYY-MM-DD` calendar-date parsing, and non-empty 1–100 char category; return a normalized `Transaction` or all per-field errors (all-or-nothing)
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.3, 3.4_

  - [x] 3.2 Write property test for date validation
    - **Property 3: Date validation accepts only valid ISO 8601 calendar dates** (accept iff valid `YYYY-MM-DD` calendar date; otherwise reject with a Transaction_Date field error)
    - **Validates: Requirements 1.3, 1.5, 2.3, 2.4**

  - [x] 3.3 Write property test for all-or-nothing edit rejection
    - **Property 5: Edit with any invalid field is an all-or-nothing rejection** (returned errors identify exactly the set of invalid fields; no normalized transaction produced)
    - **Validates: Requirements 3.4**

  - [x] 3.4 Write unit tests for missing-field validation messages
    - Explicit examples for missing amount, date, and category messages reinforcing the validation properties
    - _Requirements: 1.3, 1.4, 2.3, 2.5, 3.3_

- [x] 4. Implement aggregation (dashboard summary)
  - [x] 4.1 Implement Summary
    - Write `Summary(txns []Transaction) DashboardSummary` computing total income, total expense, net balance (income − expense, may be negative), and per-category expense breakdown (one entry per category with ≥ 1 expense, none otherwise); empty input yields zero totals
    - _Requirements: 5.1, 5.2, 5.3, 5.4_

  - [x] 4.2 Write property test for totals and net balance
    - **Property 8: Summary totals and net balance are correct** (totals equal sums; net balance = income − expense; empty set yields zeros)
    - **Validates: Requirements 5.1, 5.2, 5.3**

  - [x] 4.3 Write property test for category breakdown
    - **Property 9: Category breakdown is exact and complete** (exactly one entry per expense category, none for others; per-category totals sum to overall total expense)
    - **Validates: Requirements 5.4**

- [x] 5. Implement time-period grouping
  - [x] 5.1 Implement Group
    - Write `Group(txns []Transaction, period TimePeriod) []Group` bucketing by day / week (starting Monday) / month, ordered most-recent-first, each group carrying its own `Summary` totals and net balance
    - _Requirements: 7.1, 7.2, 7.3, 7.4_

  - [x] 5.2 Write property test for grouping partition
    - **Property 11: Grouping is an exhaustive, disjoint, ordered partition** (each transaction in exactly one correct bucket; union equals full set; groups ordered most-recent to least-recent)
    - **Validates: Requirements 7.1, 7.2, 7.3**

  - [x] 5.3 Write property test for per-group totals consistency
    - **Property 12: Per-group totals are consistent with overall totals** (each group's net = its income − its expense; group sums equal overall income/expense)
    - **Validates: Requirements 7.4**

- [x] 6. Implement sorting and filtering
  - [x] 6.1 Implement Sort and Filter
    - Write `Sort(txns []Transaction, field SortField, dir SortDir) []Transaction` returning a sorted copy by date or amount with ties broken by ascending id, defaulting to date descending
    - Write `Filter(txns []Transaction, typeFilter TypeFilter) []Transaction` returning only matching type, or all when no filter
    - _Requirements: 6.3, 6.5, 6.6, 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [x] 6.2 Write property test for sort ordering and tie-breaking
    - **Property 13: Sorting orders correctly with deterministic tie-breaking** (ordered by selected field/direction; equal values ordered by ascending id; default date descending)
    - **Validates: Requirements 6.3, 8.1, 8.2, 8.3, 8.4, 8.6**

  - [x] 6.3 Write property test for sort preserving the result set
    - **Property 14: Sorting preserves the filtered result set** (sorted output is a permutation/same multiset of the filtered input)
    - **Validates: Requirements 8.5**

  - [x] 6.4 Write property test for type filtering
    - **Property 15: Type filter returns only matching transactions** (only selected type when filtered; all types when no filter, preserving default ordering)
    - **Validates: Requirements 6.5, 6.6**

- [x] 7. Implement query-parameter parsing
  - [x] 7.1 Implement ParsePeriod and ParseSort
    - Write `ParsePeriod` returning the period for exactly `day`/`week`/`month` and defaulting to `month` otherwise
    - Write `ParseSort` returning the selection when both field and direction are recognized, falling back to date descending otherwise
    - _Requirements: 5.5, 7.7, 7.8, 8.7_

  - [x] 7.2 Write property test for period parsing
    - **Property 17: Time_Period parsing defaults to month** (selected period for exact matches; `month` for absent/unrecognized)
    - **Validates: Requirements 5.5, 7.7, 7.8**

  - [x] 7.3 Write property test for sort parsing
    - **Property 18: Sort parsing defaults to date descending** (selection when both recognized; date-descending fallback otherwise)
    - **Validates: Requirements 8.7**

- [x] 8. Checkpoint - domain layer complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 9. Implement data access layer (sqlc + SQLite)
  - [x] 9.1 Define schema and sqlc queries, generate query code
    - Add the `transactions` table schema and index, the CHECK constraints from the design, and the sqlc `.sql` query definitions for create/get/update/delete/list
    - Run sqlc generation to produce the typed query code
    - _Requirements: 9.1, 9.2_

  - [x] 9.2 Implement repository wrapper and schema bootstrap
    - Write the repository exposing `CreateTransaction`, `GetTransaction` (not-found sentinel), `UpdateTransaction`, `DeleteTransaction`, `ListTransactions`, and `EnsureSchema` (idempotent); wrap each mutation in a SQLite transaction that commits on success and rolls back on failure
    - Add mapping between stored rows and domain `Transaction` values (cents, ISO date, type)
    - _Requirements: 3.2, 4.2, 9.1, 9.2, 9.5_

  - [x] 9.3 Write property test for create round-trip
    - **Property 1: Create round-trip preserves transaction data** (persist then retrieve yields normalized input; appears in listing) — run against a temporary/in-memory SQLite database
    - **Validates: Requirements 1.1, 1.6, 2.1, 2.6**

  - [x] 9.4 Write property test for edit round-trip
    - **Property 4: Edit round-trip reflects updated values** (apply valid edit then retrieve yields edited values; listing reflects them)
    - **Validates: Requirements 3.1, 3.5**

  - [x] 9.5 Write property test for non-existent-id mutations
    - **Property 6: Mutation on a non-existent id is a no-op error** (edit/delete of absent id returns not-found; stored set unchanged)
    - **Validates: Requirements 3.2, 4.2**

  - [x] 9.6 Write property test for delete semantics
    - **Property 7: Delete removes the target and retains all others** (deleted id unretrievable; listing equals prior minus that transaction)
    - **Validates: Requirements 4.1, 4.3**

  - [x] 9.7 Write property test for durability across restart
    - **Property 19: Persistence is durable across restart** (after a sequence of successful mutations, close/reopen the store yields exactly the committed state)
    - **Validates: Requirements 9.1**

  - [x] 9.8 Write property test for atomic failed mutations
    - **Property 20: Failed mutations are atomic** (using an injectable store wrapper that forces failure, store contents equal pre-operation contents with no partial write)
    - **Validates: Requirements 9.5**

- [x] 10. Checkpoint - persistence layer complete
  - Ensure all tests pass, ask the user if questions arise.

- [x] 11. Implement presentation layer (templates)
  - [x] 11.1 Create page, dashboard, and log templates
    - Add `page.html` (full shell embedding dashboard + log), `dashboard.html` (totals, net balance, per-category breakdown, unavailable indicator), and `log.html` (rows with all fields, grouped rendering, empty-state messages)
    - Add a render helper that executes templates with auto-escaping of category/description
    - _Requirements: 5.1, 6.1, 6.2, 6.4, 7.6, 10.1_

  - [x] 11.2 Create error and 404 templates
    - Add `error.html` and `404.html` for operation-failure and not-found responses
    - _Requirements: 10.3, 10.4_

  - [x] 11.3 Write property test for log row rendering
    - **Property 16: Log rendering includes all transaction fields** (rendered row includes two-decimal amount, category, `YYYY-MM-DD` date, type, and description empty when none)
    - **Validates: Requirements 6.1, 6.2**

- [x] 12. Implement HTTP handlers and wire everything together
  - [x] 12.1 Implement routing and view handlers
    - Register routes (`GET /`, `GET /transactions`, `GET /dashboard`, and the 404 fallback); parse `period`/`type`/`sort`/`dir` params via the domain parsers; load transactions, apply filter/sort/group and summary, and render the full page or the appropriate fragment
    - Handle read/aggregation failures: totals-unavailable indicator for dashboard/group views and error (no partial rows) for log retrieval
    - _Requirements: 5.5, 6.7, 7.5, 8.5, 10.1, 10.3_

  - [x] 12.2 Implement mutation handlers
    - Implement `POST /transactions`, `POST /transactions/{id}`, and `DELETE /transactions/{id}`: bind input, call `Validate`, invoke the repository, and return re-rendered Log + Dashboard fragments; map validation errors to 422 with inline messages, not-found to 404, and store failures to 500 with data unchanged
    - _Requirements: 1.6, 2.6, 3.2, 3.5, 4.2, 4.3, 4.4, 9.5, 10.2, 10.4_

  - [x] 12.3 Implement server bootstrap and startup schema handling
    - Wire `cmd/server` to open SQLite, run `EnsureSchema`, and start serving; on schema failure enter degraded mode that rejects transaction requests with a Data_Store-unavailable error
    - _Requirements: 9.2, 9.3, 9.4_

  - [x] 12.4 Write integration and smoke tests
    - Root page returns full HTML (both dashboard and log); create/edit/delete return fragments (no `<html>` shell); 404 for unknown routes; injected Data_Store failures for delete/dashboard/log/root produce specified responses with data unchanged; schema bootstrap creates schema and accepts requests, and forced schema failure rejects requests
    - _Requirements: 4.4, 5.6, 6.7, 7.5, 9.2, 9.3, 9.4, 10.1, 10.2, 10.3, 10.4_

- [x] 13. Final checkpoint - ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional test sub-tasks and can be skipped for a faster MVP.
- Each property-based test uses `pgregory.net/rapid`, runs a minimum of 100 iterations, and carries a `// Feature: budget-tracker, Property {number}: {property_text}` comment.
- Pure-logic property tests (2, 3, 5, 8–18) run entirely in memory; persistence property tests (1, 4, 6, 7, 19, 20) run against a temporary/in-memory SQLite database, with Property 20 using an injectable failing store.
- Each task references specific granular requirements for traceability, and checkpoints ensure incremental validation.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["1.2"] },
    { "id": 2, "tasks": ["2.1", "4.1", "5.1", "6.1", "7.1"] },
    { "id": 3, "tasks": ["2.2", "2.3", "3.1", "4.2", "4.3", "5.2", "5.3", "6.2", "6.3", "6.4", "7.2", "7.3"] },
    { "id": 4, "tasks": ["3.2", "3.3", "3.4", "9.1"] },
    { "id": 5, "tasks": ["9.2", "11.1", "11.2"] },
    { "id": 6, "tasks": ["9.3", "9.4", "9.5", "9.6", "9.7", "9.8", "11.3", "12.1", "12.2", "12.3"] },
    { "id": 7, "tasks": ["12.4"] }
  ]
}
```
