package store

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	"pgregory.net/rapid"
)

// nxDBCounter provides a unique suffix for each isolated in-memory database so
// that concurrent/successive rapid iterations never share state.
var nxDBCounter int64

// nxOpenRepo opens a fresh, isolated in-memory SQLite database and ensures the
// schema exists. The connection pool is pinned to a single connection so the
// in-memory database persists for the lifetime of the returned Repo. The
// caller is responsible for closing the returned Repo.
func nxOpenRepo(t *rapid.T) *Repo {
	t.Helper()
	dsn := fmt.Sprintf("file:nxmem_%d?mode=memory&cache=shared", atomic.AddInt64(&nxDBCounter, 1))
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	// Pin to a single connection: a shared-cache in-memory database is
	// discarded once its last connection closes, so restricting the pool to
	// one connection keeps the database alive and isolated per Repo.
	repo.DB().SetMaxOpenConns(1)
	if err := repo.EnsureSchema(context.Background()); err != nil {
		repo.Close()
		t.Fatalf("ensure schema: %v", err)
	}
	return repo
}

// nxGenTxn builds a random, domain-valid transaction (id is assigned by the
// store on create, so it is left zero here). Amounts stay within the valid
// cents range and categories/dates satisfy the schema CHECK constraints.
func nxGenTxn(t *rapid.T, label string) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, label+"_isIncome") {
		typ = budget.TypeIncome
	}
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, label+"_cents")
	category := rapid.SampledFrom([]string{
		"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining",
	}).Draw(t, label+"_category")
	day := rapid.IntRange(1, 28).Draw(t, label+"_day")
	month := rapid.IntRange(1, 12).Draw(t, label+"_month")
	year := rapid.IntRange(2000, 2030).Draw(t, label+"_year")

	return budget.Transaction{
		Type:        typ,
		AmountCents: cents,
		Date:        time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
		Category:    category,
		Description: rapid.SampledFrom([]string{"", "note", "monthly"}).Draw(t, label+"_desc"),
	}
}

// nxEqualSet reports whether two transaction slices contain exactly the same
// transactions (compared by all fields, keyed by id) regardless of order.
func nxEqualSet(a, b []budget.Transaction) bool {
	if len(a) != len(b) {
		return false
	}
	index := make(map[int64]budget.Transaction, len(a))
	for _, tr := range a {
		index[tr.ID] = tr
	}
	for _, tr := range b {
		prev, ok := index[tr.ID]
		if !ok {
			return false
		}
		if prev.Type != tr.Type ||
			prev.AmountCents != tr.AmountCents ||
			!prev.Date.Equal(tr.Date) ||
			prev.Category != tr.Category ||
			prev.Description != tr.Description {
			return false
		}
	}
	return true
}

// Feature: budget-tracker, Property 6: Mutation on a non-existent id is a no-op error
func TestNonExistentMutationProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		repo := nxOpenRepo(t)
		defer repo.Close()

		// Seed a random number of transactions.
		n := rapid.IntRange(0, 20).Draw(t, "n")
		var maxID int64
		for i := 0; i < n; i++ {
			created, err := repo.CreateTransaction(ctx, nxGenTxn(t, fmt.Sprintf("seed%d", i)))
			if err != nil {
				t.Fatalf("seed create: %v", err)
			}
			if created.ID > maxID {
				maxID = created.ID
			}
		}

		// Record the stored set before attempting the bogus mutations.
		before, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("list before: %v", err)
		}

		// Build the set of existing ids and pick an id guaranteed to be absent:
		// beyond the largest assigned id by a random positive offset.
		existing := make(map[int64]bool, len(before))
		for _, tr := range before {
			existing[tr.ID] = true
		}
		offset := rapid.Int64Range(1, 1_000_000).Draw(t, "offset")
		absentID := maxID + offset
		if existing[absentID] {
			t.Fatalf("test bug: chosen absentID %d is present", absentID)
		}

		// Attempt an update against the absent id.
		editInput := nxGenTxn(t, "edit")
		editInput.ID = absentID
		_, updErr := repo.UpdateTransaction(ctx, editInput)
		if !errors.Is(updErr, ErrNotFound) {
			t.Fatalf("update on absent id %d: expected ErrNotFound, got %v", absentID, updErr)
		}

		// Attempt a delete against the absent id.
		delErr := repo.DeleteTransaction(ctx, absentID)
		if !errors.Is(delErr, ErrNotFound) {
			t.Fatalf("delete on absent id %d: expected ErrNotFound, got %v", absentID, delErr)
		}

		// The stored set must be unchanged by either failed mutation.
		after, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("list after: %v", err)
		}
		if !nxEqualSet(before, after) {
			t.Fatalf("stored set changed after no-op mutations:\nbefore=%+v\nafter=%+v", before, after)
		}
	})
}
