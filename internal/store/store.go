// Package store is the data access layer for the Budget Tracker. It wraps a
// SQLite database (via the pure-Go modernc.org/sqlite driver) with a repository
// exposing user, session, and per-user financial data operations, plus schema
// bootstrap and migration.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	// modernc.org/sqlite registers the pure-Go "sqlite" driver.
	_ "modernc.org/sqlite"
)

// isoDate is the layout used to store and parse Transaction_Date values.
const isoDate = "2006-01-02"

// ErrNotFound is returned when a referenced transaction does not exist (or is
// not owned by the requesting user).
var ErrNotFound = errors.New("store: transaction not found")

//go:embed schema.sql
var schemaSQL string

// indexSQL creates user-scoped indexes. It runs after migrations so the
// referenced columns are guaranteed to exist even on an upgraded database.
const indexSQL = `
CREATE INDEX IF NOT EXISTS idx_transactions_user_date ON transactions (user_id, date);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);
`

// Repo is the data repository backed by a *sql.DB.
type Repo struct {
	db *sql.DB
}

// New builds a Repo around an already-open *sql.DB.
func New(db *sql.DB) *Repo { return &Repo{db: db} }

// Open opens a SQLite database at the given DSN using modernc.org/sqlite.
func Open(dsn string) (*Repo, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}
	return New(db), nil
}

// DB exposes the underlying database handle.
func (r *Repo) DB() *sql.DB { return r.db }

// Close closes the underlying database handle.
func (r *Repo) Close() error { return r.db.Close() }

// EnsureSchema creates the schema if absent and migrates older databases to the
// current shape, then creates user-scoped indexes. It is idempotent.
func (r *Repo) EnsureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store: ensure schema: %w", err)
	}
	if err := r.migrate(ctx); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, indexSQL); err != nil {
		return fmt.Errorf("store: ensure indexes: %w", err)
	}
	return nil
}

// migrate upgrades an older database (one created before multi-user support) by
// adding user_id columns and rebuilding the budgets table with a composite key.
// Existing rows are assigned to user id 1 (the first/default user). On a fresh
// database these are all no-ops.
func (r *Repo) migrate(ctx context.Context) error {
	for _, table := range []string{"transactions", "recurring", "bills"} {
		has, err := r.hasColumn(ctx, table, "user_id")
		if err != nil {
			return err
		}
		if !has {
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1", table)
			if _, err := r.db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("add user_id to %s: %w", table, err)
			}
		}
	}

	has, err := r.hasColumn(ctx, "budgets", "user_id")
	if err != nil {
		return err
	}
	if !has {
		stmts := []string{
			`CREATE TABLE budgets_new (
			    user_id      INTEGER NOT NULL,
			    category     TEXT    NOT NULL CHECK (length(category) BETWEEN 1 AND 100),
			    amount_cents INTEGER NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 99999999999),
			    PRIMARY KEY (user_id, category)
			)`,
			`INSERT INTO budgets_new (user_id, category, amount_cents) SELECT 1, category, amount_cents FROM budgets`,
			`DROP TABLE budgets`,
			`ALTER TABLE budgets_new RENAME TO budgets`,
		}
		for _, s := range stmts {
			if _, err := r.db.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("rebuild budgets: %w", err)
			}
		}
	}
	return nil
}

// hasColumn reports whether the given table has a column with the given name.
func (r *Repo) hasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// withTx runs fn inside a transaction, committing on success and rolling back
// on any error or panic.
func (r *Repo) withTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: commit transaction: %w", err)
	}
	committed = true
	return nil
}

// CreateTransaction inserts a new transaction owned by userID and returns the
// stored value including its assigned ID.
func (r *Repo) CreateTransaction(ctx context.Context, userID int64, tx budget.Transaction) (budget.Transaction, error) {
	var created budget.Transaction
	err := r.withTx(ctx, func(sqlTx *sql.Tx) error {
		res, err := sqlTx.ExecContext(ctx,
			`INSERT INTO transactions (user_id, type, amount_cents, date, category, description)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			userID, string(tx.Type), tx.AmountCents, formatDate(tx.Date), tx.Category, tx.Description)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		created = tx
		created.ID = id
		return nil
	})
	if err != nil {
		return budget.Transaction{}, fmt.Errorf("store: create transaction: %w", err)
	}
	return created, nil
}

// GetTransaction returns the transaction with the given id owned by userID, or
// ErrNotFound.
func (r *Repo) GetTransaction(ctx context.Context, userID, id int64) (budget.Transaction, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, type, amount_cents, date, category, description
		 FROM transactions WHERE id = ? AND user_id = ?`, id, userID)
	tx, err := scanTxn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return budget.Transaction{}, ErrNotFound
	}
	if err != nil {
		return budget.Transaction{}, fmt.Errorf("store: get transaction: %w", err)
	}
	return tx, nil
}

// UpdateTransaction updates a transaction owned by userID and returns the stored
// value. It returns ErrNotFound if no such transaction exists for the user.
func (r *Repo) UpdateTransaction(ctx context.Context, userID int64, tx budget.Transaction) (budget.Transaction, error) {
	err := r.withTx(ctx, func(sqlTx *sql.Tx) error {
		res, err := sqlTx.ExecContext(ctx,
			`UPDATE transactions SET type = ?, amount_cents = ?, date = ?, category = ?, description = ?
			 WHERE id = ? AND user_id = ?`,
			string(tx.Type), tx.AmountCents, formatDate(tx.Date), tx.Category, tx.Description, tx.ID, userID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return budget.Transaction{}, ErrNotFound
		}
		return budget.Transaction{}, fmt.Errorf("store: update transaction: %w", err)
	}
	return tx, nil
}

// DeleteTransaction removes a transaction owned by userID, or returns
// ErrNotFound.
func (r *Repo) DeleteTransaction(ctx context.Context, userID, id int64) error {
	return r.withTx(ctx, func(sqlTx *sql.Tx) error {
		res, err := sqlTx.ExecContext(ctx,
			`DELETE FROM transactions WHERE id = ? AND user_id = ?`, id, userID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ListTransactions returns all transactions owned by userID, ordered by date
// descending then id ascending.
func (r *Repo) ListTransactions(ctx context.Context, userID int64) ([]budget.Transaction, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, amount_cents, date, category, description
		 FROM transactions WHERE user_id = ? ORDER BY date DESC, id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list transactions: %w", err)
	}
	defer rows.Close()
	out := make([]budget.Transaction, 0)
	for rows.Next() {
		tx, err := scanTxn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tx)
	}
	return out, rows.Err()
}

// scanner abstracts *sql.Row and *sql.Rows for scanning a transaction.
type scanner interface {
	Scan(dest ...any) error
}

// scanTxn scans a transaction row (id, type, amount_cents, date, category,
// description) into a domain Transaction.
func scanTxn(s scanner) (budget.Transaction, error) {
	var (
		tx      budget.Transaction
		typeStr string
		dateStr string
	)
	if err := s.Scan(&tx.ID, &typeStr, &tx.AmountCents, &dateStr, &tx.Category, &tx.Description); err != nil {
		return budget.Transaction{}, err
	}
	date, err := parseDate(dateStr)
	if err != nil {
		return budget.Transaction{}, err
	}
	tx.Type = budget.TxType(typeStr)
	tx.Date = date
	return tx, nil
}

// formatDate renders a domain date as an ISO 8601 YYYY-MM-DD string.
func formatDate(t time.Time) string { return t.UTC().Format(isoDate) }

// parseDate parses an ISO 8601 YYYY-MM-DD string into a UTC-midnight time.
func parseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(isoDate, s, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: parse date %q: %w", s, err)
	}
	return t, nil
}
