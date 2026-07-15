package http

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store"
)

// integration_test.go exercises the real HTTP handlers against a real
// *store.Repo (user-scoped), plus injected-failure scenarios. Session auth is
// bypassed via newTestServer (which injects a per-user store); the login/session
// flow itself is covered in auth_test.go.

// itUID is the user id used by the HTTP integration/feature tests.
const itUID int64 = 1

// itNewRepo opens a real SQLite store on a fresh temp-file database and ensures
// the schema.
func itNewRepo(t *testing.T) *store.Repo {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "budget.db")
	repo, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("store.Open(%q) failed: %v", dsn, err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}
	return repo
}

// itSeed inserts the given transactions for user itUID.
func itSeed(t *testing.T, repo *store.Repo, txns ...budget.Transaction) []budget.Transaction {
	t.Helper()
	out := make([]budget.Transaction, 0, len(txns))
	for _, tx := range txns {
		created, err := repo.CreateTransaction(context.Background(), itUID, tx)
		if err != nil {
			t.Fatalf("seed CreateTransaction failed: %v", err)
		}
		out = append(out, created)
	}
	return out
}

// itFailingStore wraps a per-user Store and can force errors on
// ListTransactions and DeleteTransaction, leaving the underlying data intact.
type itFailingStore struct {
	inner      Store
	failList   bool
	failDelete bool
}

func (f *itFailingStore) ListTransactions(ctx context.Context) ([]budget.Transaction, error) {
	if f.failList {
		return nil, errors.New("injected Data_Store list failure")
	}
	return f.inner.ListTransactions(ctx)
}

func (f *itFailingStore) CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return f.inner.CreateTransaction(ctx, tx)
}

func (f *itFailingStore) UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return f.inner.UpdateTransaction(ctx, tx)
}

func (f *itFailingStore) DeleteTransaction(ctx context.Context, id int64) error {
	if f.failDelete {
		return errors.New("injected Data_Store delete failure")
	}
	return f.inner.DeleteTransaction(ctx, id)
}

func (f *itFailingStore) GetTransaction(ctx context.Context, id int64) (budget.Transaction, error) {
	return f.inner.GetTransaction(ctx, id)
}

func TestIntegrationRootReturnsFullPage(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeIncome, AmountCents: 250000, Date: date(2024, 5, 13), Category: "Salary", Description: "May pay"},
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 1299, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	h := newTestServer(repo.ForUser(itUID))

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

func TestIntegrationMutationsReturnFragments(t *testing.T) {
	repo := itNewRepo(t)
	h := newTestServer(repo.ForUser(itUID))

	// --- Create ---
	createForm := url.Values{
		"type": {"expense"}, "amount": {"42.50"}, "date": {"2024-05-20"},
		"category": {"Groceries"}, "description": {"Weekly shop"},
	}
	rec := doForm(t, h, http.MethodPost, "/transactions", createForm)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("create response must be a fragment (no <html> shell):\n%s", body)
	}
	for _, want := range []string{"id=\"dashboard\"", "id=\"log\"", "Groceries", "42.50"} {
		if !strings.Contains(body, want) {
			t.Errorf("create fragment missing %q\n%s", want, body)
		}
	}

	txns, err := repo.ListTransactions(context.Background(), itUID)
	if err != nil {
		t.Fatalf("list after create failed: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 stored transaction after create, got %d", len(txns))
	}
	id := txns[0].ID

	// --- Edit ---
	editForm := url.Values{
		"type": {"expense"}, "amount": {"99.99"}, "date": {"2024-06-01"},
		"category": {"Rent"}, "description": {"June rent"},
	}
	rec = doForm(t, h, http.MethodPost, "/transactions/"+itID(id), editForm)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("edit response must be a fragment (no <html> shell):\n%s", body)
	}
	for _, want := range []string{"id=\"dashboard\"", "id=\"log\"", "Rent", "99.99"} {
		if !strings.Contains(body, want) {
			t.Errorf("edit fragment missing %q\n%s", want, body)
		}
	}
	visibleEdit := body
	if i := strings.Index(body, "<datalist"); i >= 0 {
		visibleEdit = body[:i]
	}
	if strings.Contains(visibleEdit, "Groceries") {
		t.Errorf("edit fragment should no longer show the old category:\n%s", body)
	}
	updated, err := repo.GetTransaction(context.Background(), itUID, id)
	if err != nil {
		t.Fatalf("get after edit failed: %v", err)
	}
	if updated.Category != "Rent" || updated.AmountCents != 9999 {
		t.Errorf("stored transaction not updated: %+v", updated)
	}

	// --- Delete ---
	rec = do(t, h, http.MethodDelete, "/transactions/"+itID(id))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("delete response must be a fragment (no <html> shell):\n%s", body)
	}
	visibleDel := body
	if i := strings.Index(body, "<datalist"); i >= 0 {
		visibleDel = body[:i]
	}
	if strings.Contains(visibleDel, "Rent") {
		t.Errorf("delete fragment should exclude the deleted transaction:\n%s", body)
	}
	remaining, err := repo.ListTransactions(context.Background(), itUID)
	if err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected store empty after delete, got %d", len(remaining))
	}
}

func TestIntegrationUnknownRouteReturns404(t *testing.T) {
	repo := itNewRepo(t)
	h := newTestServer(repo.ForUser(itUID))

	rec := do(t, h, http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
		t.Errorf("expected not-found page, got:\n%s", rec.Body.String())
	}
}

func TestIntegrationRootRenderFailureLeavesDataUnchanged(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 500, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	failing := &itFailingStore{inner: repo.ForUser(itUID), failList: true}
	h := newTestServer(failing)

	rec := do(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "id=\"log\"") || strings.Contains(body, "Groceries") {
		t.Errorf("error page must not include partial content:\n%s", body)
	}
	if !strings.Contains(body, "could not be loaded") {
		t.Errorf("expected root-load error message, got:\n%s", body)
	}

	failing.failList = false
	txns, err := repo.ListTransactions(context.Background(), itUID)
	if err != nil {
		t.Fatalf("list after failure failed: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("stored data changed after render failure: got %d transactions", len(txns))
	}
}

func TestIntegrationLogRetrievalFailureNoPartialRows(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 500, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	failing := &itFailingStore{inner: repo.ForUser(itUID), failList: true}
	h := newTestServer(failing)

	rec := do(t, h, http.MethodGet, "/transactions")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Groceries") {
		t.Errorf("log error response must not contain partial rows:\n%s", body)
	}
	if !strings.Contains(body, "could not be loaded") {
		t.Errorf("expected log-load error message, got:\n%s", body)
	}
}

func TestIntegrationDashboardTotalsUnavailable(t *testing.T) {
	repo := itNewRepo(t)
	failing := &itFailingStore{inner: repo.ForUser(itUID), failList: true}
	h := newTestServer(failing)

	rec := do(t, h, http.MethodGet, "/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected totals-unavailable indicator, got:\n%s", rec.Body.String())
	}
}

func TestIntegrationDeleteStoreFailurePreservesTarget(t *testing.T) {
	repo := itNewRepo(t)
	seeded := itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 1000, Date: date(2024, 5, 1), Category: "Keep"},
	)
	failing := &itFailingStore{inner: repo.ForUser(itUID), failDelete: true}
	h := newTestServer(failing)

	rec := do(t, h, http.MethodDelete, "/transactions/"+itID(seeded[0].ID))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500\n%s", rec.Code, rec.Body.String())
	}

	got, err := repo.GetTransaction(context.Background(), itUID, seeded[0].ID)
	if err != nil {
		t.Fatalf("target transaction was not preserved after delete failure: %v", err)
	}
	if got.Category != "Keep" {
		t.Errorf("target transaction altered after delete failure: %+v", got)
	}
}

func TestIntegrationDegradedModeRejectsRequests(t *testing.T) {
	h := NewDegradedHandler()

	cases := []struct{ method, target string }{
		{http.MethodGet, "/"},
		{http.MethodPost, "/transactions"},
		{http.MethodDelete, "/transactions/1"},
	}
	for _, tc := range cases {
		rec := do(t, h, tc.method, tc.target)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s status = %d, want 503", tc.method, tc.target, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Data_Store is unavailable") {
			t.Errorf("%s %s missing Data_Store-unavailable message:\n%s", tc.method, tc.target, rec.Body.String())
		}
	}
}

// itID renders a transaction id as a decimal path segment.
func itID(id int64) string {
	return strconv.FormatInt(id, 10)
}
