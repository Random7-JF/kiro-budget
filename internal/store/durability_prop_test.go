package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	"pgregory.net/rapid"
)

// durGenTxn generates a valid, normalized transaction ready to be persisted.
// The ID is left zero (assigned by the store). The date is generated at UTC
// midnight so it round-trips exactly through the date-only ISO storage format.
func durGenTxn(t *rapid.T, label string) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, label+":isIncome") {
		typ = budget.TypeIncome
	}

	// Valid amount magnitude: 1 .. 99,999,999,999 cents (0.01 .. 999,999,999.99).
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, label+":amountCents")

	// Valid ISO calendar date at UTC midnight over a wide range.
	year := rapid.IntRange(1970, 2100).Draw(t, label+":year")
	month := rapid.IntRange(1, 12).Draw(t, label+":month")
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := rapid.IntRange(1, lastDay).Draw(t, label+":day")
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	// Category: 1-100 characters drawn from Unicode letters and numbers, which
	// round-trip exactly through the TEXT storage column.
	catRunes := rapid.RuneFrom(nil, unicode.L, unicode.N)
	category := rapid.StringOfN(catRunes, 1, 100, -1).Draw(t, label+":category")

	// Description is optional (may be empty), also from letters/numbers so it
	// round-trips exactly.
	description := rapid.StringOfN(catRunes, 0, 100, -1).Draw(t, label+":description")

	return budget.Transaction{
		Type:        typ,
		AmountCents: cents,
		Date:        date,
		Category:    category,
		Description: description,
	}
}

// durTxnEqual reports whether two transactions match on every observable field,
// including the assigned ID.
func durTxnEqual(want, got budget.Transaction) bool {
	return want.ID == got.ID &&
		want.Type == got.Type &&
		want.AmountCents == got.AmountCents &&
		want.Date.Equal(got.Date) &&
		want.Category == got.Category &&
		want.Description == got.Description
}

// durListEqual reports whether two listings contain exactly the same
// transactions in the same order.
func durListEqual(want, got []budget.Transaction) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if !durTxnEqual(want[i], got[i]) {
			return false
		}
	}
	return true
}

// Feature: budget-tracker, Property 19: Persistence is durable across restart
func TestDurabilityAcrossRestartProperty(t *testing.T) {
	// A real on-disk directory (not :memory:) is required to exercise
	// durability across a restart. rapid.T does not expose TempDir, so the
	// base directory is taken from the outer *testing.T and each iteration
	// uses a uniquely-named file within it.
	baseDir := t.TempDir()
	iteration := 0
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()

		// Unique file path per iteration so state survives close/reopen while
		// remaining isolated from other iterations.
		iteration++
		dsn := filepath.Join(baseDir, fmt.Sprintf("db-%d.sqlite", iteration))

		repo1, err := Open(dsn)
		if err != nil {
			t.Fatalf("open repo1: %v", err)
		}
		if err := repo1.EnsureSchema(ctx); err != nil {
			_ = repo1.Close()
			t.Fatalf("ensure schema repo1: %v", err)
		}

		// committed tracks the expected durable state: id -> transaction.
		committed := make(map[int64]budget.Transaction)
		// ids preserves insertion order of live ids so edits/deletes can pick
		// an existing target.
		var ids []int64

		numOps := rapid.IntRange(0, 25).Draw(t, "numOps")
		for i := 0; i < numOps; i++ {
			label := fmt.Sprintf("op%d", i)

			// Choose an operation. Edits and deletes are only possible when at
			// least one transaction currently exists.
			var op string
			if len(ids) == 0 {
				op = "create"
			} else {
				op = rapid.SampledFrom([]string{"create", "edit", "delete"}).Draw(t, label+":op")
			}

			switch op {
			case "create":
				input := durGenTxn(t, label)
				created, err := repo1.CreateTransaction(ctx, input)
				if err != nil {
					t.Fatalf("%s create: %v", label, err)
				}
				committed[created.ID] = created
				ids = append(ids, created.ID)

			case "edit":
				idx := rapid.IntRange(0, len(ids)-1).Draw(t, label+":editIdx")
				id := ids[idx]
				input := durGenTxn(t, label)
				input.ID = id
				updated, err := repo1.UpdateTransaction(ctx, input)
				if err != nil {
					t.Fatalf("%s update(id=%d): %v", label, id, err)
				}
				committed[id] = updated

			case "delete":
				idx := rapid.IntRange(0, len(ids)-1).Draw(t, label+":delIdx")
				id := ids[idx]
				if err := repo1.DeleteTransaction(ctx, id); err != nil {
					t.Fatalf("%s delete(id=%d): %v", label, id, err)
				}
				delete(committed, id)
				ids = append(ids[:idx], ids[idx+1:]...)
			}
		}

		// Capture the final listing from repo1 (the committed state pre-restart).
		list1, err := repo1.ListTransactions(ctx)
		if err != nil {
			_ = repo1.Close()
			t.Fatalf("list repo1: %v", err)
		}

		// The pre-restart listing must already reflect exactly the committed set.
		if len(list1) != len(committed) {
			_ = repo1.Close()
			t.Fatalf("repo1 listing size %d != committed size %d", len(list1), len(committed))
		}
		for _, tr := range list1 {
			want, ok := committed[tr.ID]
			if !ok {
				_ = repo1.Close()
				t.Fatalf("repo1 listing contains unexpected id %d", tr.ID)
			}
			if !durTxnEqual(want, tr) {
				_ = repo1.Close()
				t.Fatalf("repo1 listing mismatch for id %d:\n want=%+v\n got =%+v", tr.ID, want, tr)
			}
		}

		// Restart: close the store and reopen against the SAME file.
		if err := repo1.Close(); err != nil {
			t.Fatalf("close repo1: %v", err)
		}

		repo2, err := Open(dsn)
		if err != nil {
			t.Fatalf("open repo2: %v", err)
		}
		defer func() { _ = repo2.Close() }()

		// EnsureSchema is idempotent and safe to run on every startup.
		if err := repo2.EnsureSchema(ctx); err != nil {
			t.Fatalf("ensure schema repo2: %v", err)
		}

		list2, err := repo2.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("list repo2: %v", err)
		}

		// The reopened store must yield exactly the committed state: identical
		// to repo1's final listing in both set and contents.
		if !durListEqual(list1, list2) {
			t.Fatalf("durability mismatch across restart:\n before=%+v\n after =%+v", list1, list2)
		}
	})
}
