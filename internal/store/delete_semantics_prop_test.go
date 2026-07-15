package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	"pgregory.net/rapid"
)

// delOpenRepo opens a fresh, isolated file-backed SQLite database in a unique
// temporary directory and ensures the schema exists. A unique temp-file DSN
// per call guarantees each rapid iteration runs against its own clean database
// (an in-memory shared-cache DSN can leak state between opens). The directory
// is removed when the rapid check completes via t.Cleanup.
func delOpenRepo(t *rapid.T, ctx context.Context) *Repo {
	dir, err := os.MkdirTemp("", "delete_semantics")
	if err != nil {
		t.Fatalf("delOpenRepo: mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	dsn := filepath.Join(dir, "delete_semantics.db")

	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("delOpenRepo: open: %v", err)
	}
	if err := repo.EnsureSchema(ctx); err != nil {
		_ = repo.Close()
		t.Fatalf("delOpenRepo: ensure schema: %v", err)
	}
	return repo
}

// delGenTxn generates a valid, normalized transaction ready to be persisted.
// The ID is left zero (assigned by the store). The date is generated at UTC
// midnight so it round-trips exactly through the date-only ISO storage format.
func delGenTxn(t *rapid.T) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = budget.TypeIncome
	}

	// Valid amount magnitude: 1 .. 99,999,999,999 cents (0.01 .. 999,999,999.99).
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, "amountCents")

	// Valid ISO calendar date at UTC midnight over a wide range.
	year := rapid.IntRange(1970, 2100).Draw(t, "year")
	month := rapid.IntRange(1, 12).Draw(t, "month")
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := rapid.IntRange(1, lastDay).Draw(t, "day")
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	// Category: 1-100 characters (Unicode-capable letters/numbers).
	category := rapid.StringOfN(rapid.RuneFrom(nil, unicode.L, unicode.N), 1, 100, -1).Draw(t, "category")

	// Description is optional (may be empty).
	description := rapid.String().Draw(t, "description")

	return budget.Transaction{
		Type:        typ,
		AmountCents: cents,
		Date:        date,
		Category:    category,
		Description: description,
	}
}

// delTxnEqual reports whether two transactions match on every observable
// field, including the assigned id.
func delTxnEqual(want, got budget.Transaction) bool {
	return want.ID == got.ID &&
		want.Type == got.Type &&
		want.AmountCents == got.AmountCents &&
		want.Date.Equal(got.Date) &&
		want.Category == got.Category &&
		want.Description == got.Description
}

// Feature: budget-tracker, Property 7: Delete removes the target and retains all others
func TestDeleteSemanticsProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()

		repo := delOpenRepo(t, ctx)
		defer func() { _ = repo.Close() }()

		// Seed N >= 1 random transactions and record their assigned ids.
		n := rapid.IntRange(1, 8).Draw(t, "numTxns")
		ids := make([]int64, 0, n)
		for i := 0; i < n; i++ {
			created, err := repo.CreateTransaction(ctx, delGenTxn(t))
			if err != nil {
				t.Fatalf("CreateTransaction: %v", err)
			}
			ids = append(ids, created.ID)
		}

		// Record the stored set prior to deletion.
		before, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("ListTransactions (before): %v", err)
		}
		if len(before) != n {
			t.Fatalf("expected %d stored transactions before delete, got %d", n, len(before))
		}

		// Pick a random existing id to delete.
		idx := rapid.IntRange(0, len(ids)-1).Draw(t, "deleteIdx")
		deletedID := ids[idx]

		if err := repo.DeleteTransaction(ctx, deletedID); err != nil {
			t.Fatalf("DeleteTransaction(%d): %v", deletedID, err)
		}

		// Requirement 4.1: the deleted transaction is no longer retrievable.
		if _, err := repo.GetTransaction(ctx, deletedID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetTransaction(%d) after delete: expected ErrNotFound, got %v", deletedID, err)
		}

		// Requirement 4.3: the resulting listing equals the prior listing with
		// exactly that one transaction removed (all others retained).
		after, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("ListTransactions (after): %v", err)
		}

		// Build the expected listing: the prior listing minus the deleted id.
		expected := make([]budget.Transaction, 0, len(before))
		removed := 0
		for _, tr := range before {
			if tr.ID == deletedID {
				removed++
				continue
			}
			expected = append(expected, tr)
		}
		if removed != 1 {
			t.Fatalf("expected exactly one prior transaction with id %d, found %d", deletedID, removed)
		}

		if len(after) != len(expected) {
			t.Fatalf("listing length after delete = %d, want %d", len(after), len(expected))
		}

		// The store lists in a deterministic order (date desc, id asc), so the
		// prior listing with the target removed must match element-for-element.
		for i := range expected {
			if !delTxnEqual(expected[i], after[i]) {
				t.Fatalf("listing mismatch at index %d after delete:\n want=%+v\n got =%+v", i, expected[i], after[i])
			}
		}

		// Defensively confirm the deleted id is absent and every retained id
		// is still present.
		for _, tr := range after {
			if tr.ID == deletedID {
				t.Fatalf("deleted id %d still present in listing", deletedID)
			}
		}
		for _, id := range ids {
			if id == deletedID {
				continue
			}
			if _, err := repo.GetTransaction(ctx, id); err != nil {
				t.Fatalf("retained transaction id %d unexpectedly unretrievable: %v", id, err)
			}
		}
	})
}
