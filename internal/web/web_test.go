package web

import (
	"strings"
	"testing"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

func sampleTxns() []budget.Transaction {
	return []budget.Transaction{
		{ID: 1, Type: budget.TypeIncome, AmountCents: 250000, Date: date(2024, 5, 13), Category: "Salary", Description: "May pay"},
		{ID: 2, Type: budget.TypeExpense, AmountCents: 1299, Date: date(2024, 5, 14), Category: "Groceries", Description: ""},
	}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestRenderDashboard(t *testing.T) {
	view := DashboardView{Summary: budget.Summary(sampleTxns())}
	var sb strings.Builder
	if err := Render(&sb, "dashboard", view); err != nil {
		t.Fatalf("render dashboard: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"2500.00", "12.99", "2487.01", "Groceries"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard output missing %q\n%s", want, out)
		}
	}
}

func TestRenderDashboardTotalsUnavailable(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "dashboard", DashboardView{TotalsUnavailable: true}); err != nil {
		t.Fatalf("render dashboard: %v", err)
	}
	if !strings.Contains(sb.String(), "unavailable") {
		t.Errorf("expected unavailable indicator, got:\n%s", sb.String())
	}
}

func TestRenderLogFlatIncludesAllFields(t *testing.T) {
	var sb strings.Builder
	if err := Render(&sb, "log", LogView{Items: sampleTxns()}); err != nil {
		t.Fatalf("render log: %v", err)
	}
	out := sb.String()
	// Amount two decimals, category, ISO date, type, description present.
	for _, want := range []string{"2500.00", "12.99", "Salary", "Groceries", "2024-05-13", "2024-05-14", "income", "expense", "May pay"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q\n%s", want, out)
		}
	}
}

func TestRenderLogEmptyStates(t *testing.T) {
	var flat strings.Builder
	if err := Render(&flat, "log", LogView{}); err != nil {
		t.Fatalf("render flat log: %v", err)
	}
	if !strings.Contains(flat.String(), "No transactions") {
		t.Errorf("expected flat empty-state, got:\n%s", flat.String())
	}

	var grouped strings.Builder
	if err := Render(&grouped, "log", LogView{Grouped: true}); err != nil {
		t.Fatalf("render grouped log: %v", err)
	}
	if !strings.Contains(grouped.String(), "No transactions for this period") {
		t.Errorf("expected grouped empty-state, got:\n%s", grouped.String())
	}
}

func TestRenderLogGrouped(t *testing.T) {
	groups := budget.GroupByPeriod(sampleTxns(), budget.PeriodMonth)
	var sb strings.Builder
	if err := Render(&sb, "log", LogView{Grouped: true, Groups: groups}); err != nil {
		t.Fatalf("render grouped log: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"May 2024", "Salary", "Groceries"} {
		if !strings.Contains(out, want) {
			t.Errorf("grouped log missing %q\n%s", want, out)
		}
	}
}

func TestRenderPageIsFullDocument(t *testing.T) {
	view := PageView{
		Dashboard: DashboardView{Summary: budget.Summary(sampleTxns())},
		Log:       LogView{Items: sampleTxns()},
	}
	var sb strings.Builder
	if err := Render(&sb, "page", view); err != nil {
		t.Fatalf("render page: %v", err)
	}
	out := sb.String()
	for _, want := range []string{"<!DOCTYPE html>", "<html", "htmx.org", "id=\"dashboard\"", "id=\"log\""} {
		if !strings.Contains(out, want) {
			t.Errorf("page output missing %q\n%s", want, out)
		}
	}
}

func TestRenderAutoEscapesUserValues(t *testing.T) {
	txns := []budget.Transaction{
		{ID: 1, Type: budget.TypeExpense, AmountCents: 100, Date: date(2024, 1, 1),
			Category: "<script>", Description: "a&b\"c"},
	}
	var sb strings.Builder
	if err := Render(&sb, "log", LogView{Items: txns}); err != nil {
		t.Fatalf("render log: %v", err)
	}
	out := sb.String()
	if strings.Contains(out, "<script>") {
		t.Errorf("category was not escaped:\n%s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Errorf("expected escaped category, got:\n%s", out)
	}
}
