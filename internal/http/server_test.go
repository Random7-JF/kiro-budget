package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// fakeStore is an in-memory Store implementation for handler tests. When
// listErr is set, ListTransactions returns it to simulate a read failure.
type fakeStore struct {
	txns    []budget.Transaction
	listErr error
}

func (f *fakeStore) ListTransactions(ctx context.Context) ([]budget.Transaction, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	// Return a copy so callers cannot mutate the backing slice.
	out := make([]budget.Transaction, len(f.txns))
	copy(out, f.txns)
	return out, nil
}

func (f *fakeStore) CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return budget.Transaction{}, errors.New("not implemented in 12.1 tests")
}

func (f *fakeStore) UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return budget.Transaction{}, errors.New("not implemented in 12.1 tests")
}

func (f *fakeStore) DeleteTransaction(ctx context.Context, id int64) error {
	return errors.New("not implemented in 12.1 tests")
}

func (f *fakeStore) GetTransaction(ctx context.Context, id int64) (budget.Transaction, error) {
	return budget.Transaction{}, errors.New("not implemented in 12.1 tests")
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func sampleTxns() []budget.Transaction {
	return []budget.Transaction{
		{ID: 1, Type: budget.TypeIncome, AmountCents: 250000, Date: date(2024, 5, 13), Category: "Salary", Description: "May pay"},
		{ID: 2, Type: budget.TypeExpense, AmountCents: 1299, Date: date(2024, 5, 14), Category: "Groceries"},
	}
}

func newTestServer(store Store) http.Handler {
	return NewServer(store).Routes()
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRootReturnsFullPageWithDashboardAndLog(t *testing.T) {
	h := newTestServer(&fakeStore{txns: sampleTxns()})
	rec := do(t, h, http.MethodGet, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<html", "id=\"dashboard\"", "id=\"log\"", "Salary", "Groceries"} {
		if !strings.Contains(body, want) {
			t.Errorf("root page missing %q\n%s", want, body)
		}
	}
}

func TestRootReadFailureRendersErrorNoPartialRows(t *testing.T) {
	h := newTestServer(&fakeStore{listErr: errors.New("boom")})
	rec := do(t, h, http.MethodGet, "/")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "id=\"log\"") {
		t.Errorf("error page should not include the log fragment:\n%s", body)
	}
	if !strings.Contains(body, "could not be loaded") {
		t.Errorf("expected error message, got:\n%s", body)
	}
}

func TestTransactionsReturnsLogFragment(t *testing.T) {
	h := newTestServer(&fakeStore{txns: sampleTxns()})
	rec := do(t, h, http.MethodGet, "/transactions")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("log fragment must not contain a full page shell:\n%s", body)
	}
	for _, want := range []string{"Salary", "Groceries"} {
		if !strings.Contains(body, want) {
			t.Errorf("log fragment missing %q\n%s", want, body)
		}
	}
}

func TestTransactionsReadFailureReturnsErrorNoPartialRows(t *testing.T) {
	h := newTestServer(&fakeStore{listErr: errors.New("boom")})
	rec := do(t, h, http.MethodGet, "/transactions")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Salary") || strings.Contains(body, "Groceries") {
		t.Errorf("error response must not contain partial rows:\n%s", body)
	}
	if !strings.Contains(body, "could not be loaded") {
		t.Errorf("expected log-load error message, got:\n%s", body)
	}
}

func TestDashboardReturnsDashboardFragment(t *testing.T) {
	h := newTestServer(&fakeStore{txns: sampleTxns()})
	rec := do(t, h, http.MethodGet, "/dashboard")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("dashboard fragment must not contain a full page shell:\n%s", body)
	}
	// Totals: income 2500.00, expense 12.99, net 2487.01.
	for _, want := range []string{"2500.00", "12.99", "2487.01"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard fragment missing %q\n%s", want, body)
		}
	}
}

func TestDashboardReadFailureShowsTotalsUnavailable(t *testing.T) {
	h := newTestServer(&fakeStore{listErr: errors.New("boom")})
	rec := do(t, h, http.MethodGet, "/dashboard")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected totals-unavailable indicator, got:\n%s", rec.Body.String())
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	h := newTestServer(&fakeStore{txns: sampleTxns()})
	rec := do(t, h, http.MethodGet, "/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not found") &&
		!strings.Contains(rec.Body.String(), "Not found") {
		t.Errorf("expected not-found page, got:\n%s", rec.Body.String())
	}
}
