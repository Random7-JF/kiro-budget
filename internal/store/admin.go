package store

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed seed.sql
var seedSQL string

// resetTables lists every data table cleared by Reset and Seed, ordered so the
// operation is safe to run repeatedly. sqlite_sequence is cleared last to reset
// AUTOINCREMENT counters.
var resetTables = []string{
	"recurring_postings",
	"transactions",
	"budgets",
	"recurring",
	"bills",
	"sqlite_sequence",
}

// Seed replaces all data with the built-in demo dataset (see seed.sql). It
// first clears existing data, so applying it always yields the same known
// state. Intended for local development and testing.
func (r *Repo) Seed(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, seedSQL); err != nil {
		return fmt.Errorf("store: seed: %w", err)
	}
	return nil
}

// Reset deletes all data from every table (transactions, budgets, recurring
// rules and their posting ledger, and bills) and resets AUTOINCREMENT counters.
// The schema itself is left intact. This is a destructive operation intended
// for testing.
func (r *Repo) Reset(ctx context.Context) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin reset: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, table := range resetTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("store: reset %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit reset: %w", err)
	}
	committed = true
	return nil
}
