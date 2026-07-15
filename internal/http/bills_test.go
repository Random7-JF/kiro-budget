package http

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// TestFeatureBillCreateListDelete verifies bill management endpoints persist and
// redirect, and that the breakdown appears on the page.
func TestFeatureBillCreateListDelete(t *testing.T) {
	repo := itNewRepo(t)
	h := newTestServer(repo.ForUser(itUID))

	rec := doForm(t, h, http.MethodPost, "/bills", url.Values{
		"payee": {"Electric Company"}, "amount": {"120.00"}, "frequency": {"monthly"},
		"category": {"Utilities"}, "due_day": {"15"}, "month": {"2026-07"}, "billperiod": {"month"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create bill status = %d, want 303", rec.Code)
	}
	bills, err := repo.ListBills(context.Background(), itUID)
	if err != nil || len(bills) != 1 {
		t.Fatalf("bill not persisted: %+v (err %v)", bills, err)
	}
	if bills[0].Payee != "Electric Company" || bills[0].AmountCents != 12000 || bills[0].DueDay != 15 {
		t.Fatalf("bill stored incorrectly: %+v", bills[0])
	}

	rec = do(t, h, http.MethodGet, "/?billperiod=month")
	body := rec.Body.String()
	for _, want := range []string{"Electric Company", "120.00", "Estimated Monthly bills"} {
		if !strings.Contains(body, want) {
			t.Errorf("bills breakdown missing %q", want)
		}
	}

	rec = doForm(t, h, http.MethodPost, "/bills/delete", url.Values{
		"id": {itID(bills[0].ID)}, "month": {"2026-07"}, "billperiod": {"month"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete bill status = %d, want 303", rec.Code)
	}
	bills, _ = repo.ListBills(context.Background(), itUID)
	if len(bills) != 0 {
		t.Errorf("bill not deleted: %+v", bills)
	}
}

// TestFeatureBillBreakdownPeriods verifies the breakdown total normalizes to
// the selected period.
func TestFeatureBillBreakdownPeriods(t *testing.T) {
	repo := itNewRepo(t)
	bill := budget.Bill{Payee: "Annual Insurance", AmountCents: 120000, Frequency: budget.FreqYearly, Category: "Insurance"}
	if _, err := repo.CreateBill(context.Background(), itUID, bill); err != nil {
		t.Fatalf("CreateBill: %v", err)
	}
	h := newTestServer(repo.ForUser(itUID))

	monthly := do(t, h, http.MethodGet, "/?billperiod=month").Body.String()
	if !strings.Contains(monthly, "100.00") {
		t.Errorf("monthly breakdown should show 100.00:\n%s", monthly)
	}
	yearly := do(t, h, http.MethodGet, "/?billperiod=year").Body.String()
	if !strings.Contains(yearly, "1200.00") {
		t.Errorf("yearly breakdown should show 1200.00:\n%s", yearly)
	}
}
