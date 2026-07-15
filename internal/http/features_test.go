package http

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// TestFeatureMonthScopedDashboard verifies the Overview totals reflect the
// selected month rather than all-time.
func TestFeatureMonthScopedDashboard(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 5000, Date: date(2026, 6, 10), Category: "Groceries"},
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 7000, Date: date(2026, 7, 10), Category: "Groceries"},
	)
	h := newTestServer(repo.ForUser(itUID))

	rec := do(t, h, http.MethodGet, "/dashboard?month=2026-06")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "50.00") {
		t.Errorf("June dashboard should show 50.00:\n%s", body)
	}
	if strings.Contains(body, "70.00") {
		t.Errorf("June dashboard must not include July's 70.00:\n%s", body)
	}
	if !strings.Contains(body, "June 2026") {
		t.Errorf("expected June 2026 label:\n%s", body)
	}

	rec = do(t, h, http.MethodGet, "/dashboard?month=2026-07")
	if !strings.Contains(rec.Body.String(), "70.00") || strings.Contains(rec.Body.String(), "50.00") {
		t.Errorf("July dashboard scoping wrong:\n%s", rec.Body.String())
	}
}

// TestFeatureBudgetsProgress verifies a configured budget shows progress on the
// month-scoped dashboard.
func TestFeatureBudgetsProgress(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 8000, Date: date(2026, 7, 5), Category: "Groceries"},
	)
	if err := repo.SetBudget(context.Background(), itUID, "Groceries", 10000); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	h := newTestServer(repo.ForUser(itUID))

	rec := do(t, h, http.MethodGet, "/dashboard?month=2026-07")
	body := rec.Body.String()
	for _, want := range []string{"budget-row", "80.00", "100.00", "80%"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard budget progress missing %q:\n%s", want, body)
		}
	}
}

// TestFeatureBudgetSetAndDelete verifies the budget management endpoints persist
// and redirect.
func TestFeatureBudgetSetAndDelete(t *testing.T) {
	repo := itNewRepo(t)
	h := newTestServer(repo.ForUser(itUID))

	rec := doForm(t, h, http.MethodPost, "/budgets", url.Values{
		"category": {"Dining"}, "amount": {"200.00"}, "month": {"2026-07"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("set budget status = %d, want 303", rec.Code)
	}
	budgets, err := repo.ListBudgets(context.Background(), itUID)
	if err != nil || len(budgets) != 1 || budgets[0].Category != "Dining" || budgets[0].LimitCents != 20000 {
		t.Fatalf("budget not persisted: %+v (err %v)", budgets, err)
	}

	rec = doForm(t, h, http.MethodPost, "/budgets/delete", url.Values{
		"category": {"Dining"}, "month": {"2026-07"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete budget status = %d, want 303", rec.Code)
	}
	budgets, _ = repo.ListBudgets(context.Background(), itUID)
	if len(budgets) != 0 {
		t.Errorf("budget not deleted: %+v", budgets)
	}
}

// TestFeatureRecurringCreateAndPost verifies creating a recurring rule and
// posting it for a month materializes a transaction idempotently.
func TestFeatureRecurringCreateAndPost(t *testing.T) {
	repo := itNewRepo(t)
	h := newTestServer(repo.ForUser(itUID))

	rec := doForm(t, h, http.MethodPost, "/recurring", url.Values{
		"type": {"expense"}, "amount": {"1350.00"}, "category": {"Rent"},
		"description": {"Monthly rent"}, "day": {"1"}, "month": {"2026-07"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create recurring status = %d, want 303", rec.Code)
	}
	rules, err := repo.ListRecurring(context.Background(), itUID)
	if err != nil || len(rules) != 1 {
		t.Fatalf("recurring not persisted: %+v (err %v)", rules, err)
	}

	rec = doForm(t, h, http.MethodPost, "/recurring/post", url.Values{"month": {"2026-07"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("post recurring status = %d, want 303", rec.Code)
	}
	txns, _ := repo.ListTransactions(context.Background(), itUID)
	if len(txns) != 1 || txns[0].Category != "Rent" || txns[0].AmountCents != 135000 {
		t.Fatalf("recurring not materialized: %+v", txns)
	}
	if got := txns[0].Date.Format("2006-01-02"); got != "2026-07-01" {
		t.Errorf("recurring date = %q, want 2026-07-01", got)
	}

	rec = doForm(t, h, http.MethodPost, "/recurring/post", url.Values{"month": {"2026-07"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("second post status = %d, want 303", rec.Code)
	}
	txns, _ = repo.ListTransactions(context.Background(), itUID)
	if len(txns) != 1 {
		t.Errorf("expected idempotent post (1 txn), got %d", len(txns))
	}
}
