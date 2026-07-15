package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"

	"pgregory.net/rapid"
)

// erEdit bundles the initial transaction to create and the edited values to
// apply to it. The edited values reuse the created transaction's ID so the
// edit targets an existing row.
type erEdit struct {
	initial budget.Transaction
	edited  budget.Transaction
}

// erOpenRepo opens a fresh, isolated in-memory SQLite database and ensures the
// schema exists. Each rapid iteration gets its own uniquely-named shared-cache
// in-memory database, so state never leaks between iterations. The connection
// pool is capped at a single connection so the named in-memory database stays
// alive for the lifetime of the repo. The repo is closed automatically when
// the test completes.
func erOpenRepo(t *rapid.T) *Repo {
	// A unique database name per iteration keeps in-memory databases isolated.
	name := "er_" + rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(t, "erDBName")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)

	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("erOpenRepo: open: %v", err)
	}
	// Keep a single connection so the named in-memory DB persists across calls.
	repo.DB().SetMaxOpenConns(1)
	t.Cleanup(func() { _ = repo.Close() })

	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("erOpenRepo: ensure schema: %v", err)
	}
	return repo
}

// erGenType generates a transaction type (expense or income).
func erGenType(t *rapid.T, label string) budget.TxType {
	return rapid.SampledFrom([]budget.TxType{budget.TypeExpense, budget.TypeIncome}).Draw(t, label)
}

// erGenAmountCents generates a valid amount magnitude in cents within the
// permitted range 0.01 .. 999,999,999.99 (i.e. 1 .. 99,999,999,999 cents).
func erGenAmountCents(t *rapid.T, label string) int64 {
	return rapid.Int64Range(1, 99_999_999_999).Draw(t, label)
}

// erGenDate generates a valid calendar date at UTC midnight, matching how the
// store persists and parses dates (YYYY-MM-DD, UTC).
func erGenDate(t *rapid.T, label string) time.Time {
	base := time.Date(1000, time.January, 1, 0, 0, 0, 0, time.UTC)
	offset := rapid.IntRange(0, 2_900_000).Draw(t, label)
	return base.AddDate(0, 0, offset)
}

// erGenCategory generates a non-empty category label of 1..100 characters.
func erGenCategory(t *rapid.T, label string) string {
	return rapid.StringMatching(`[\p{L}\p{N} ]{1,100}`).Draw(t, label)
}

// erGenDescription generates an optional description (may be empty).
func erGenDescription(t *rapid.T, label string) string {
	return rapid.StringMatching(`[\p{L}\p{N} ]{0,200}`).Draw(t, label)
}

// erGenTxn generates a valid domain Transaction (ID left as zero; assigned by
// the store on create). The prefix disambiguates the draw labels.
func erGenTxn(t *rapid.T, prefix string) budget.Transaction {
	return budget.Transaction{
		Type:        erGenType(t, prefix+"Type"),
		AmountCents: erGenAmountCents(t, prefix+"Amount"),
		Date:        erGenDate(t, prefix+"Date"),
		Category:    erGenCategory(t, prefix+"Category"),
		Description: erGenDescription(t, prefix+"Description"),
	}
}

// erGenEdit generates an initial transaction and an independent set of valid
// edited values to apply to it.
func erGenEdit() *rapid.Generator[erEdit] {
	return rapid.Custom(func(t *rapid.T) erEdit {
		return erEdit{
			initial: erGenTxn(t, "init"),
			edited:  erGenTxn(t, "edit"),
		}
	})
}

// erFindByID returns the transaction with the given id from a listing, and
// whether it was found.
func erFindByID(txns []budget.Transaction, id int64) (budget.Transaction, bool) {
	for _, tx := range txns {
		if tx.ID == id {
			return tx, true
		}
	}
	return budget.Transaction{}, false
}

// erSameValues reports whether two transactions carry equal user-visible field
// values (ignoring ID, which is set by the store). Dates are compared as
// date-only UTC values since that is the persisted granularity.
func erSameValues(a, b budget.Transaction) bool {
	return a.Type == b.Type &&
		a.AmountCents == b.AmountCents &&
		a.Date.UTC().Format(isoDate) == b.Date.UTC().Format(isoDate) &&
		a.Category == b.Category &&
		a.Description == b.Description
}

// Feature: budget-tracker, Property 4: Edit round-trip reflects updated values
func TestEditRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		repo := erOpenRepo(t)
		ctx := context.Background()

		spec := erGenEdit().Draw(t, "edit")

		// Create the initial transaction.
		created, err := repo.CreateTransaction(ctx, testUID, spec.initial)
		if err != nil {
			t.Fatalf("CreateTransaction: %v", err)
		}

		// Apply a valid edit that keeps the same ID but changes the values.
		edit := spec.edited
		edit.ID = created.ID

		updated, err := repo.UpdateTransaction(ctx, testUID, edit)
		if err != nil {
			t.Fatalf("UpdateTransaction: %v", err)
		}
		if updated.ID != created.ID {
			t.Fatalf("UpdateTransaction changed ID: got %d, want %d", updated.ID, created.ID)
		}
		if !erSameValues(updated, edit) {
			t.Fatalf("UpdateTransaction returned mismatched values:\n got  %+v\n want %+v", updated, edit)
		}

		// Retrieving the transaction yields the edited values.
		got, err := repo.GetTransaction(ctx, testUID, created.ID)
		if err != nil {
			t.Fatalf("GetTransaction: %v", err)
		}
		if got.ID != created.ID {
			t.Fatalf("GetTransaction returned wrong ID: got %d, want %d", got.ID, created.ID)
		}
		if !erSameValues(got, edit) {
			t.Fatalf("GetTransaction did not reflect edit:\n got  %+v\n want %+v", got, edit)
		}

		// The listing also reflects the edited values.
		list, err := repo.ListTransactions(ctx, testUID)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		listed, ok := erFindByID(list, created.ID)
		if !ok {
			t.Fatalf("ListTransactions did not include edited transaction id %d", created.ID)
		}
		if !erSameValues(listed, edit) {
			t.Fatalf("ListTransactions did not reflect edit:\n got  %+v\n want %+v", listed, edit)
		}
	})
}
