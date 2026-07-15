package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	"pgregory.net/rapid"
)

// crDBCounter yields a unique suffix per opened database so each rapid
// iteration is isolated from every other.
var crDBCounter int64

// crOpenRepo opens a fresh, isolated file-backed SQLite database in a unique
// temp directory and ensures the schema exists. A per-call temp-file DSN
// guarantees each rapid iteration runs against its own clean database (a
// shared-cache in-memory DSN can leak state between opens). The returned
// cleanup function closes the repo and removes the temp directory.
func crOpenRepo(t *rapid.T, ctx context.Context) (*Repo, func()) {
	n := atomic.AddInt64(&crDBCounter, 1)
	dir, err := os.MkdirTemp("", fmt.Sprintf("cr_roundtrip_%d_", n))
	if err != nil {
		t.Fatalf("crOpenRepo: mkdir temp: %v", err)
	}
	dsn := filepath.Join(dir, "create_roundtrip.db")

	repo, err := Open(dsn)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("crOpenRepo: open: %v", err)
	}
	if err := repo.EnsureSchema(ctx); err != nil {
		_ = repo.Close()
		_ = os.RemoveAll(dir)
		t.Fatalf("crOpenRepo: ensure schema: %v", err)
	}

	cleanup := func() {
		_ = repo.Close()
		_ = os.RemoveAll(dir)
	}
	return repo, cleanup
}

// crGenTxn generates a valid, normalized transaction ready to be persisted.
// The ID is left zero (assigned by the store). The date is generated at UTC
// midnight so it round-trips exactly through the date-only ISO storage format.
func crGenTxn(t *rapid.T) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = budget.TypeIncome
	}

	// Valid amount magnitude: 1 .. 99,999,999,999 cents (0.01 .. 999,999,999.99).
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, "amountCents")

	// Valid ISO calendar date at UTC midnight over a wide range.
	year := rapid.IntRange(1970, 2100).Draw(t, "year")
	month := rapid.IntRange(1, 12).Draw(t, "month")
	// Pick a day valid for the drawn month/year.
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := rapid.IntRange(1, lastDay).Draw(t, "day")
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	// crSafeRune draws printable runes across Latin, Greek, and Cyrillic letter
	// ranges. Control characters (including NUL) are excluded because they are
	// not realistic label/description content and can obscure the round-trip
	// property with storage-encoding noise.
	safeRune := rapid.RuneFrom(nil, unicode.Latin, unicode.Greek, unicode.Cyrillic)

	// Category: 1-100 characters (Unicode-capable via rune generation).
	category := rapid.StringOfN(safeRune, 1, 100, -1).Draw(t, "category")

	// Description is optional (may be empty).
	description := rapid.StringOfN(safeRune, 0, 200, -1).Draw(t, "description")

	return budget.Transaction{
		Type:        typ,
		AmountCents: cents,
		Date:        date,
		Category:    category,
		Description: description,
	}
}

// crTxnEqual reports whether the stored transaction matches the normalized
// input on every observable field (id-independent).
func crTxnEqual(want, got budget.Transaction) bool {
	return want.Type == got.Type &&
		want.AmountCents == got.AmountCents &&
		want.Date.Equal(got.Date) &&
		want.Category == got.Category &&
		want.Description == got.Description
}

// Feature: budget-tracker, Property 1: Create round-trip preserves transaction data
func TestCreateRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()

		repo, cleanup := crOpenRepo(t, ctx)
		defer cleanup()

		input := crGenTxn(t)

		created, err := repo.CreateTransaction(ctx, testUID, input)
		if err != nil {
			t.Fatalf("CreateTransaction: %v", err)
		}
		if created.ID == 0 {
			t.Fatalf("CreateTransaction: expected a non-zero assigned ID")
		}
		if !crTxnEqual(input, created) {
			t.Fatalf("CreateTransaction returned mismatched data:\n input=%+v\n got  =%+v", input, created)
		}

		// Persisting then retrieving yields the normalized input.
		got, err := repo.GetTransaction(ctx, testUID, created.ID)
		if err != nil {
			t.Fatalf("GetTransaction(%d): %v", created.ID, err)
		}
		if got.ID != created.ID {
			t.Fatalf("GetTransaction returned id %d, want %d", got.ID, created.ID)
		}
		if !crTxnEqual(input, got) {
			t.Fatalf("GetTransaction returned mismatched data:\n input=%+v\n got  =%+v", input, got)
		}

		// The persisted transaction appears in the listing.
		list, err := repo.ListTransactions(ctx, testUID)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		found := false
		for _, tr := range list {
			if tr.ID == created.ID {
				found = true
				if !crTxnEqual(input, tr) {
					t.Fatalf("ListTransactions entry mismatched data:\n input=%+v\n got  =%+v", input, tr)
				}
				break
			}
		}
		if !found {
			t.Fatalf("ListTransactions did not include created transaction id %d", created.ID)
		}
	})
}
