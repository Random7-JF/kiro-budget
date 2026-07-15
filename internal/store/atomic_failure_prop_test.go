package store

import (
	"context"
	"database/sql"
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

// afDBCounter yields a unique suffix per opened database so each rapid
// iteration is isolated from every other.
var afDBCounter int64

// afOpenRepo opens a fresh, isolated file-backed SQLite database through the
// failing driver, ensures the schema exists, and returns the repo, its
// controllable failure flag, and a cleanup function. Each call uses a unique
// temp-file DSN so iterations never share state, and the failure flag is bound
// to that DSN.
func afOpenRepo(t *rapid.T, ctx context.Context) (*Repo, *afFailFlag, func()) {
	afEnsureDriver()

	n := atomic.AddInt64(&afDBCounter, 1)
	dir, err := os.MkdirTemp("", fmt.Sprintf("af_atomic_%d_", n))
	if err != nil {
		t.Fatalf("afOpenRepo: mkdir temp: %v", err)
	}
	dsn := filepath.Join(dir, "atomic_failure.db")

	flag := afNewFlag(dsn)

	db, err := sql.Open(afDriverName, dsn)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("afOpenRepo: open: %v", err)
	}
	repo := New(db)
	if err := repo.EnsureSchema(ctx); err != nil {
		_ = repo.Close()
		_ = os.RemoveAll(dir)
		t.Fatalf("afOpenRepo: ensure schema: %v", err)
	}

	cleanup := func() {
		_ = repo.Close()
		_ = os.RemoveAll(dir)
	}
	return repo, flag, cleanup
}

// afGenTxn generates a valid, normalized transaction ready to be persisted.
// The ID is left zero (assigned by the store). The date is generated at UTC
// midnight so it round-trips exactly through the date-only ISO storage format.
func afGenTxn(t *rapid.T, label string) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, label+":isIncome") {
		typ = budget.TypeIncome
	}

	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, label+":amountCents")

	year := rapid.IntRange(1970, 2100).Draw(t, label+":year")
	month := rapid.IntRange(1, 12).Draw(t, label+":month")
	lastDay := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := rapid.IntRange(1, lastDay).Draw(t, label+":day")
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

	catRunes := rapid.RuneFrom(nil, unicode.L, unicode.N)
	category := rapid.StringOfN(catRunes, 1, 100, -1).Draw(t, label+":category")
	description := rapid.StringOfN(catRunes, 0, 100, -1).Draw(t, label+":description")

	return budget.Transaction{
		Type:        typ,
		AmountCents: cents,
		Date:        date,
		Category:    category,
		Description: description,
	}
}

// afTxnEqual reports whether two transactions match on every observable field,
// including the assigned ID.
func afTxnEqual(want, got budget.Transaction) bool {
	return want.ID == got.ID &&
		want.Type == got.Type &&
		want.AmountCents == got.AmountCents &&
		want.Date.Equal(got.Date) &&
		want.Category == got.Category &&
		want.Description == got.Description
}

// afListEqual reports whether two listings contain exactly the same
// transactions in the same order.
func afListEqual(want, got []budget.Transaction) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if !afTxnEqual(want[i], got[i]) {
			return false
		}
	}
	return true
}

// Feature: budget-tracker, Property 20: Failed mutations are atomic
func TestAtomicFailedMutationProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()

		repo, flag, cleanup := afOpenRepo(t, ctx)
		defer cleanup()

		// Seed a random set of transactions successfully (failure disarmed).
		seedCount := rapid.IntRange(0, 12).Draw(t, "seedCount")
		var ids []int64
		for i := 0; i < seedCount; i++ {
			input := afGenTxn(t, fmt.Sprintf("seed%d", i))
			created, err := repo.CreateTransaction(ctx, input)
			if err != nil {
				t.Fatalf("seed %d create: %v", i, err)
			}
			ids = append(ids, created.ID)
		}

		// Snapshot the pre-operation store contents.
		before, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("snapshot ListTransactions: %v", err)
		}

		// Choose which mutation to attempt. Edit and delete require an existing
		// row; create is always possible.
		mutation := "create"
		if len(ids) > 0 {
			mutation = rapid.SampledFrom([]string{"create", "edit", "delete"}).Draw(t, "mutation")
		}

		// Choose where the underlying Data_Store failure is injected: on the
		// statement itself, or at transaction commit time. Both leave the store
		// unchanged when rollback is correct.
		failMode := rapid.SampledFrom([]string{"op", "commit"}).Draw(t, "failMode")
		switch failMode {
		case "op":
			flag.armOp()
		case "commit":
			flag.armCommit()
		}

		// Attempt the mutation; it MUST fail because the Data_Store operation
		// has been forced to fail mid-transaction.
		var mutErr error
		switch mutation {
		case "create":
			input := afGenTxn(t, "mut")
			_, mutErr = repo.CreateTransaction(ctx, input)
		case "edit":
			idx := rapid.IntRange(0, len(ids)-1).Draw(t, "editIdx")
			input := afGenTxn(t, "mut")
			input.ID = ids[idx]
			_, mutErr = repo.UpdateTransaction(ctx, input)
		case "delete":
			idx := rapid.IntRange(0, len(ids)-1).Draw(t, "delIdx")
			mutErr = repo.DeleteTransaction(ctx, ids[idx])
		}

		// Disarm so the verifying read succeeds against the real data.
		flag.disarm()

		if mutErr == nil {
			t.Fatalf("mutation %q with fail mode %q unexpectedly succeeded; expected an injected failure", mutation, failMode)
		}

		// The store contents must equal the pre-operation snapshot exactly:
		// rollback left no partial write.
		after, err := repo.ListTransactions(ctx)
		if err != nil {
			t.Fatalf("post-failure ListTransactions: %v", err)
		}
		if !afListEqual(before, after) {
			t.Fatalf("failed %q mutation (mode %q) was not atomic:\n before=%+v\n after =%+v", mutation, failMode, before, after)
		}
	})
}
