package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTempRepo(t *testing.T) *Repo {
	t.Helper()
	repo, err := Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return repo
}

func TestSeedPopulatesAllTables(t *testing.T) {
	repo := newTempRepo(t)
	ctx := context.Background()

	if err := repo.Seed(ctx); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	txns, _ := repo.ListTransactions(ctx)
	budgets, _ := repo.ListBudgets(ctx)
	recurring, _ := repo.ListRecurring(ctx)
	bills, _ := repo.ListBills(ctx)

	if len(txns) == 0 || len(budgets) == 0 || len(recurring) == 0 || len(bills) == 0 {
		t.Fatalf("seed left a table empty: txns=%d budgets=%d recurring=%d bills=%d",
			len(txns), len(budgets), len(recurring), len(bills))
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	repo := newTempRepo(t)
	ctx := context.Background()

	if err := repo.Seed(ctx); err != nil {
		t.Fatalf("first Seed: %v", err)
	}
	first, _ := repo.ListTransactions(ctx)

	if err := repo.Seed(ctx); err != nil {
		t.Fatalf("second Seed: %v", err)
	}
	second, _ := repo.ListTransactions(ctx)

	if len(first) != len(second) {
		t.Errorf("seed not idempotent: first=%d second=%d transactions", len(first), len(second))
	}
}

func TestResetClearsAllData(t *testing.T) {
	repo := newTempRepo(t)
	ctx := context.Background()

	if err := repo.Seed(ctx); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if err := repo.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	txns, _ := repo.ListTransactions(ctx)
	budgets, _ := repo.ListBudgets(ctx)
	recurring, _ := repo.ListRecurring(ctx)
	bills, _ := repo.ListBills(ctx)
	if len(txns) != 0 || len(budgets) != 0 || len(recurring) != 0 || len(bills) != 0 {
		t.Errorf("reset did not clear all data: txns=%d budgets=%d recurring=%d bills=%d",
			len(txns), len(budgets), len(recurring), len(bills))
	}

	// After reset, seeding again should work and re-use id 1 (AUTOINCREMENT reset).
	if err := repo.Seed(ctx); err != nil {
		t.Fatalf("Seed after reset: %v", err)
	}
	txns, _ = repo.ListTransactions(ctx)
	if len(txns) == 0 {
		t.Error("expected transactions after re-seed")
	}
}
