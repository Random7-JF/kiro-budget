package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed seed.sql
var seedSQL string

// dataTables lists the per-user financial tables (for reassigning seeded rows).
var dataTables = []string{"transactions", "budgets", "recurring", "bills"}

// resetTables lists every table cleared by Reset, ordered so the operation is
// safe. Users and sessions are intentionally preserved.
var resetTables = []string{
	"recurring_postings",
	"transactions",
	"budgets",
	"recurring",
	"bills",
}

// Seed replaces all financial data with the built-in demo dataset (see
// seed.sql) and assigns it to userID. Existing users and sessions are
// preserved. Intended for local development and testing.
func (r *Repo) Seed(ctx context.Context, userID int64) error {
	if _, err := r.db.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("store: seed: %w", err)
	}
	// seed.sql inserts rows with a placeholder user_id of 0; assign them to the
	// target user.
	for _, table := range dataTables {
		if _, err := r.db.ExecContext(ctx, "UPDATE "+table+" SET user_id = ? WHERE user_id = 0", userID); err != nil {
			return fmt.Errorf("store: seed assign %s: %w", table, err)
		}
	}
	return nil
}

// Reset deletes all financial data (transactions, budgets, recurring rules and
// their posting ledger, and bills) for every user. Users and sessions are
// preserved and the schema is left intact. This is a destructive operation
// intended for testing.
func (r *Repo) Reset(ctx context.Context) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		for _, table := range resetTables {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return fmt.Errorf("store: reset %s: %w", table, err)
			}
		}
		return nil
	})
}
