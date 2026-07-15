// Package store is the data access layer for the Budget Tracker. It wraps
// sqlc-generated query code against a SQLite database and provides schema
// bootstrap plus a repository exposing transaction CRUD operations.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store/sqlcgen"

	// modernc.org/sqlite registers the pure-Go "sqlite" driver.
	_ "modernc.org/sqlite"
)

// isoDate is the layout used to store and parse Transaction_Date values.
// Dates are persisted as ISO 8601 YYYY-MM-DD strings, which sort
// lexicographically in calendar order.
const isoDate = "2006-01-02"

// ErrNotFound is returned by GetTransaction, UpdateTransaction, and
// DeleteTransaction when the referenced transaction does not exist.
var ErrNotFound = errors.New("store: transaction not found")

//go:embed schema.sql
var schemaSQL string

// Repo is the transaction repository. It wraps a *sql.DB together with the
// sqlc-generated queries bound to that database.
type Repo struct {
	db      *sql.DB
	queries *sqlcgen.Queries
}

// New builds a Repo around an already-open *sql.DB.
func New(db *sql.DB) *Repo {
	return &Repo{
		db:      db,
		queries: sqlcgen.New(db),
	}
}

// Open opens a SQLite database at the given DSN using the pure-Go
// modernc.org/sqlite driver and returns a Repo wrapping it.
func Open(dsn string) (*Repo, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}
	return New(db), nil
}

// DB exposes the underlying database handle (useful for closing or for
// callers that need direct access).
func (r *Repo) DB() *sql.DB {
	return r.db
}

// Close closes the underlying database handle.
func (r *Repo) Close() error {
	return r.db.Close()
}

// EnsureSchema creates the required schema if it is absent. The embedded
// schema uses CREATE TABLE/INDEX IF NOT EXISTS, so it is idempotent and safe
// to run on every startup.
func (r *Repo) EnsureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: ensure schema: %w", err)
	}
	return nil
}

// CreateTransaction inserts a new transaction and returns the stored domain
// value including its assigned ID. The insert runs inside a SQLite
// transaction that commits on success and rolls back on failure.
func (r *Repo) CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	var created budget.Transaction
	err := r.withTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.CreateTransaction(ctx, sqlcgen.CreateTransactionParams{
			Type:        string(tx.Type),
			AmountCents: tx.AmountCents,
			Date:        formatDate(tx.Date),
			Category:    tx.Category,
			Description: tx.Description,
		})
		if err != nil {
			return err
		}
		created, err = toDomain(row)
		return err
	})
	if err != nil {
		return budget.Transaction{}, err
	}
	return created, nil
}

// GetTransaction returns the transaction with the given id, or ErrNotFound if
// no such transaction exists.
func (r *Repo) GetTransaction(ctx context.Context, id int64) (budget.Transaction, error) {
	row, err := r.queries.GetTransaction(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return budget.Transaction{}, ErrNotFound
	}
	if err != nil {
		return budget.Transaction{}, fmt.Errorf("store: get transaction: %w", err)
	}
	return toDomain(row)
}

// UpdateTransaction updates an existing transaction and returns the stored
// domain value. It returns ErrNotFound if the transaction does not exist. The
// update runs inside a SQLite transaction that commits on success and rolls
// back on failure.
func (r *Repo) UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	var updated budget.Transaction
	err := r.withTx(ctx, func(q *sqlcgen.Queries) error {
		row, err := q.UpdateTransaction(ctx, sqlcgen.UpdateTransactionParams{
			Type:        string(tx.Type),
			AmountCents: tx.AmountCents,
			Date:        formatDate(tx.Date),
			Category:    tx.Category,
			Description: tx.Description,
			ID:          tx.ID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		updated, err = toDomain(row)
		return err
	})
	if err != nil {
		return budget.Transaction{}, err
	}
	return updated, nil
}

// DeleteTransaction removes the transaction with the given id. It returns
// ErrNotFound if the transaction does not exist. The delete runs inside a
// SQLite transaction that commits on success and rolls back on failure.
func (r *Repo) DeleteTransaction(ctx context.Context, id int64) error {
	return r.withTx(ctx, func(q *sqlcgen.Queries) error {
		// Confirm the row exists within the same transaction so a missing id
		// is reported as ErrNotFound rather than a silent no-op.
		if _, err := q.GetTransaction(ctx, id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		return q.DeleteTransaction(ctx, id)
	})
}

// ListTransactions returns all stored transactions as domain values, ordered
// by date descending then id ascending (per the underlying query).
func (r *Repo) ListTransactions(ctx context.Context) ([]budget.Transaction, error) {
	rows, err := r.queries.ListTransactions(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list transactions: %w", err)
	}
	out := make([]budget.Transaction, 0, len(rows))
	for _, row := range rows {
		t, err := toDomain(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// withTx runs fn inside a SQLite transaction, committing on success and
// rolling back on any error. The queries passed to fn are bound to the
// transaction so all statements share the same atomic unit of work.
func (r *Repo) withTx(ctx context.Context, fn func(q *sqlcgen.Queries) error) (err error) {
	sqlTx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = sqlTx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = sqlTx.Rollback()
		}
	}()

	if err = fn(r.queries.WithTx(sqlTx)); err != nil {
		return err
	}
	if err = sqlTx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	return nil
}

// toDomain maps a stored row to a domain Transaction, parsing the ISO date.
func toDomain(row sqlcgen.Transaction) (budget.Transaction, error) {
	date, err := parseDate(row.Date)
	if err != nil {
		return budget.Transaction{}, err
	}
	return budget.Transaction{
		ID:          row.ID,
		Type:        budget.TxType(row.Type),
		AmountCents: row.AmountCents,
		Date:        date,
		Category:    row.Category,
		Description: row.Description,
	}, nil
}

// formatDate renders a domain date as an ISO 8601 YYYY-MM-DD string.
func formatDate(t time.Time) string {
	return t.UTC().Format(isoDate)
}

// parseDate parses an ISO 8601 YYYY-MM-DD string into a UTC-midnight time.
func parseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(isoDate, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse date %q: %w", s, err)
	}
	return t, nil
}
