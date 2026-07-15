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

// integration_test.go exercises the REAL HTTP stack (a real *store.Repo backed
// by an on-disk SQLite database wired through Server.Routes()), plus
// injected-failure scenarios via itFailingStore. These are example-based
// integration/smoke tests using standard Go testing, not property tests.
//
// Helpers here are prefixed with `it` to avoid clashing with the shared helpers
// declared in server_test.go / mutations_test.go (do, doForm, date, etc.),
// which are reused where convenient.

// itNewRepo opens a real SQLite store on a fresh temp-file database, ensures
// the schema, and registers cleanup. The returned *store.Repo satisfies the
// Store interface used by the handlers.
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

// itSeed inserts the given transactions into the repo, returning the stored
// values (with assigned IDs).
func itSeed(t *testing.T, repo *store.Repo, txns ...budget.Transaction) []budget.Transaction {
	t.Helper()
	out := make([]budget.Transaction, 0, len(txns))
	for _, tx := range txns {
		created, err := repo.CreateTransaction(context.Background(), tx)
		if err != nil {
			t.Fatalf("seed CreateTransaction failed: %v", err)
		}
		out = append(out, created)
	}
	return out
}

// itFailingStore wraps a real Store and can be told to force errors on
// ListTransactions and DeleteTransaction, leaving the underlying data intact.
// This lets the tests inject Data_Store failures while still being able to
// verify (by toggling the failure off) that stored data is unchanged.
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

// TestIntegrationRootReturnsFullPage verifies that GET / against a real store
// returns a complete HTML document rendering both the Dashboard and the
// Transaction Log, including the seeded transaction data (Requirement 10.1).
func TestIntegrationRootReturnsFullPage(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeIncome, AmountCents: 250000, Date: date(2024, 5, 13), Category: "Salary", Description: "May pay"},
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 1299, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	h := NewServer(repo).Routes()

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

// TestIntegrationMutationsReturnFragments verifies that create, edit, and
// delete against a real store each return an HTMX fragment (no <html> shell)
// re-rendering the dashboard and log, and that the data change is reflected
// both in the response and in the store (Requirement 10.2).
func TestIntegrationMutationsReturnFragments(t *testing.T) {
	repo := itNewRepo(t)
	h := NewServer(repo).Routes()

	// --- Create ---
	createForm := url.Values{
		"type":        {"expense"},
		"amount":      {"42.50"},
		"date":        {"2024-05-20"},
		"category":    {"Groceries"},
		"description": {"Weekly shop"},
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

	// Recover the assigned id from the store.
	txns, err := repo.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("list after create failed: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 stored transaction after create, got %d", len(txns))
	}
	id := txns[0].ID

	// --- Edit ---
	editForm := url.Values{
		"type":        {"expense"},
		"amount":      {"99.99"},
		"date":        {"2024-06-01"},
		"category":    {"Rent"},
		"description": {"June rent"},
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
	// Exclude the category-suggestion datalist, which legitimately still lists
	// default categories like "Groceries"; the old category must be gone from
	// the rendered dashboard and log.
	visibleEdit := body
	if i := strings.Index(body, "<datalist"); i >= 0 {
		visibleEdit = body[:i]
	}
	if strings.Contains(visibleEdit, "Groceries") {
		t.Errorf("edit fragment should no longer show the old category:\n%s", body)
	}
	updated, err := repo.GetTransaction(context.Background(), id)
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
	for _, want := range []string{"id=\"dashboard\"", "id=\"log\""} {
		if !strings.Contains(body, want) {
			t.Errorf("delete fragment missing %q\n%s", want, body)
		}
	}
	// Exclude the suggestion datalist (which still lists the default "Rent"
	// category); the deleted transaction must be gone from the log.
	visibleDel := body
	if i := strings.Index(body, "<datalist"); i >= 0 {
		visibleDel = body[:i]
	}
	if strings.Contains(visibleDel, "Rent") {
		t.Errorf("delete fragment should exclude the deleted transaction:\n%s", body)
	}
	remaining, err := repo.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected store empty after delete, got %d", len(remaining))
	}
}

// TestIntegrationUnknownRouteReturns404 verifies an unknown route returns HTTP
// 404 with the not-found error page (Requirement 10.3).
func TestIntegrationUnknownRouteReturns404(t *testing.T) {
	repo := itNewRepo(t)
	h := NewServer(repo).Routes()

	rec := do(t, h, http.MethodGet, "/nope")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
		t.Errorf("expected not-found page, got:\n%s", rec.Body.String())
	}
}

// TestIntegrationRootRenderFailureLeavesDataUnchanged verifies that when the
// root page cannot be rendered because the store read fails, the handler
// returns HTTP 500 with an error page and no partial log content, and the
// stored data is unchanged (Requirement 10.4).
func TestIntegrationRootRenderFailureLeavesDataUnchanged(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 500, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	failing := &itFailingStore{inner: repo, failList: true}
	h := NewServer(failing).Routes()

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

	// Data must be unchanged: turn the injected failure off and confirm the
	// seeded transaction is still present.
	failing.failList = false
	txns, err := repo.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("list after failure failed: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("stored data changed after render failure: got %d transactions", len(txns))
	}
}

// TestIntegrationLogRetrievalFailureNoPartialRows verifies that when the log
// cannot be retrieved because the store read fails, the handler returns an
// error with no partial transaction rows (Requirement 6.7).
func TestIntegrationLogRetrievalFailureNoPartialRows(t *testing.T) {
	repo := itNewRepo(t)
	itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 500, Date: date(2024, 5, 14), Category: "Groceries"},
	)
	failing := &itFailingStore{inner: repo, failList: true}
	h := NewServer(failing).Routes()

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

// TestIntegrationDashboardTotalsUnavailable verifies that when the dashboard
// totals cannot be computed because the store read fails, the handler returns
// HTTP 200 with a totals-unavailable indicator (Requirements 5.6, 7.5).
func TestIntegrationDashboardTotalsUnavailable(t *testing.T) {
	repo := itNewRepo(t)
	failing := &itFailingStore{inner: repo, failList: true}
	h := NewServer(failing).Routes()

	rec := do(t, h, http.MethodGet, "/dashboard")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected totals-unavailable indicator, got:\n%s", rec.Body.String())
	}
}

// TestIntegrationDeleteStoreFailurePreservesTarget verifies that when the
// Data_Store delete operation fails, the handler returns HTTP 500 indicating
// the deletion did not complete and the target transaction is preserved
// (Requirement 4.4).
func TestIntegrationDeleteStoreFailurePreservesTarget(t *testing.T) {
	repo := itNewRepo(t)
	seeded := itSeed(t, repo,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 1000, Date: date(2024, 5, 1), Category: "Keep"},
	)
	failing := &itFailingStore{inner: repo, failDelete: true}
	h := NewServer(failing).Routes()

	rec := do(t, h, http.MethodDelete, "/transactions/"+itID(seeded[0].ID))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500\n%s", rec.Code, rec.Body.String())
	}

	// Target must be preserved.
	got, err := repo.GetTransaction(context.Background(), seeded[0].ID)
	if err != nil {
		t.Fatalf("target transaction was not preserved after delete failure: %v", err)
	}
	if got.Category != "Keep" {
		t.Errorf("target transaction altered after delete failure: %+v", got)
	}
}

// TestIntegrationSchemaBootstrapSmoke verifies that opening a fresh database
// and running EnsureSchema lets the server accept requests: GET / returns 200
// and a create is durably persisted (Requirements 9.2, 9.3).
func TestIntegrationSchemaBootstrapSmoke(t *testing.T) {
	// itNewRepo already exercises Open + EnsureSchema against a fresh temp DB.
	repo := itNewRepo(t)
	h := NewServer(repo).Routes()

	// The server accepts requests immediately after schema bootstrap.
	rec := do(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200 after schema bootstrap", rec.Code)
	}

	// A create succeeds and is persisted.
	form := url.Values{
		"type":     {"income"},
		"amount":   {"1500.00"},
		"date":     {"2024-07-01"},
		"category": {"Salary"},
	}
	rec = doForm(t, h, http.MethodPost, "/transactions", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}

	txns, err := repo.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("list after create failed: %v", err)
	}
	if len(txns) != 1 || txns[0].Category != "Salary" || txns[0].AmountCents != 150000 {
		t.Errorf("created transaction not persisted as expected: %+v", txns)
	}

	// It is also reflected in a fresh root render.
	rec = do(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Salary") {
		t.Errorf("persisted transaction not reflected on root page (status %d):\n%s", rec.Code, rec.Body.String())
	}
}

// TestIntegrationDegradedModeRejectsRequests verifies the degraded handler that
// cmd/server mounts when schema creation fails: every representative route
// returns HTTP 503 with a Data_Store-unavailable message (Requirement 9.4).
func TestIntegrationDegradedModeRejectsRequests(t *testing.T) {
	h := NewDegradedHandler()

	cases := []struct {
		method string
		target string
	}{
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
