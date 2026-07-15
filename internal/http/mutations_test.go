package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store"
)

// mutStore is a mutation-capable in-memory Store for the mutation handler
// tests. Unlike server_test.go's fakeStore (whose mutation methods return "not
// implemented"), this fake performs real create/update/delete/get against an
// in-memory slice and can be told to force a store failure on any operation.
type mutStore struct {
	txns   []budget.Transaction
	nextID int64

	listErr   error
	createErr error
	updateErr error
	deleteErr error
}

func (m *mutStore) ListTransactions(ctx context.Context) ([]budget.Transaction, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	out := make([]budget.Transaction, len(m.txns))
	copy(out, m.txns)
	return out, nil
}

func (m *mutStore) CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	if m.createErr != nil {
		return budget.Transaction{}, m.createErr
	}
	m.nextID++
	tx.ID = m.nextID
	m.txns = append(m.txns, tx)
	return tx, nil
}

func (m *mutStore) UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	if m.updateErr != nil {
		return budget.Transaction{}, m.updateErr
	}
	for i := range m.txns {
		if m.txns[i].ID == tx.ID {
			m.txns[i] = tx
			return tx, nil
		}
	}
	return budget.Transaction{}, store.ErrNotFound
}

func (m *mutStore) DeleteTransaction(ctx context.Context, id int64) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for i := range m.txns {
		if m.txns[i].ID == id {
			m.txns = append(m.txns[:i], m.txns[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mutStore) GetTransaction(ctx context.Context, id int64) (budget.Transaction, error) {
	for _, t := range m.txns {
		if t.ID == id {
			return t, nil
		}
	}
	return budget.Transaction{}, store.ErrNotFound
}

// doForm issues a form-encoded request against the handler and returns the
// recorded response.
func doForm(t *testing.T, h http.Handler, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func validCreateForm() url.Values {
	return url.Values{
		"type":        {"expense"},
		"amount":      {"42.50"},
		"date":        {"2024-05-20"},
		"category":    {"Groceries"},
		"description": {"Weekly shop"},
	}
}

func TestCreateValidReturnsFragments(t *testing.T) {
	fake := &mutStore{}
	h := newTestServer(fake)

	rec := doForm(t, h, http.MethodPost, "/transactions", validCreateForm())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") {
		t.Errorf("mutation response must not contain a full page shell:\n%s", body)
	}
	// Combined fragment: dashboard section followed by log section.
	for _, want := range []string{"id=\"dashboard\"", "id=\"log\"", "Groceries", "42.50"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n%s", want, body)
		}
	}
	if len(fake.txns) != 1 {
		t.Errorf("expected 1 stored transaction, got %d", len(fake.txns))
	}
}

func TestCreateInvalidReturns422WithFieldMessage(t *testing.T) {
	fake := &mutStore{}
	h := newTestServer(fake)

	form := validCreateForm()
	form.Set("amount", "not-a-number")

	rec := doForm(t, h, http.MethodPost, "/transactions", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-field=\"amount\"") {
		t.Errorf("expected inline amount field error, got:\n%s", body)
	}
	// Data must be unchanged on a validation rejection.
	if len(fake.txns) != 0 {
		t.Errorf("expected no stored transactions after validation error, got %d", len(fake.txns))
	}
}

func TestUpdateNonExistentReturns404(t *testing.T) {
	fake := &mutStore{}
	h := newTestServer(fake)

	rec := doForm(t, h, http.MethodPost, "/transactions/999", validCreateForm())

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
		t.Errorf("expected not-found message, got:\n%s", rec.Body.String())
	}
}

func TestUpdateValidReturnsFragments(t *testing.T) {
	fake := &mutStore{
		txns:   []budget.Transaction{{ID: 1, Type: budget.TypeExpense, AmountCents: 1000, Date: date(2024, 5, 1), Category: "Old"}},
		nextID: 1,
	}
	h := newTestServer(fake)

	form := validCreateForm()
	form.Set("category", "Updated")
	rec := doForm(t, h, http.MethodPost, "/transactions/1", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Updated") {
		t.Errorf("expected updated category in fragment, got:\n%s", body)
	}
	if fake.txns[0].Category != "Updated" {
		t.Errorf("stored category = %q, want Updated", fake.txns[0].Category)
	}
}

func TestDeleteExistingReturnsFragmentsExcludingIt(t *testing.T) {
	fake := &mutStore{
		txns: []budget.Transaction{
			{ID: 1, Type: budget.TypeExpense, AmountCents: 1000, Date: date(2024, 5, 1), Category: "DeleteMe"},
			{ID: 2, Type: budget.TypeIncome, AmountCents: 5000, Date: date(2024, 5, 2), Category: "KeepMe"},
		},
		nextID: 2,
	}
	h := newTestServer(fake)

	rec := do(t, h, http.MethodDelete, "/transactions/1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "DeleteMe") {
		t.Errorf("deleted transaction should be excluded, got:\n%s", body)
	}
	if !strings.Contains(body, "KeepMe") {
		t.Errorf("remaining transaction should be retained, got:\n%s", body)
	}
	if len(fake.txns) != 1 || fake.txns[0].ID != 2 {
		t.Errorf("expected only transaction 2 to remain, got %+v", fake.txns)
	}
}

func TestDeleteNonExistentReturns404(t *testing.T) {
	fake := &mutStore{}
	h := newTestServer(fake)

	rec := do(t, h, http.MethodDelete, "/transactions/999")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404\n%s", rec.Code, rec.Body.String())
	}
}

func TestCreateStoreFailureReturns500(t *testing.T) {
	fake := &mutStore{createErr: errors.New("boom")}
	h := newTestServer(fake)

	rec := doForm(t, h, http.MethodPost, "/transactions", validCreateForm())

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500\n%s", rec.Code, rec.Body.String())
	}
	if len(fake.txns) != 0 {
		t.Errorf("data must be unchanged on store failure, got %d transactions", len(fake.txns))
	}
}

func TestDeleteStoreFailureReturns500PreservesTarget(t *testing.T) {
	fake := &mutStore{
		txns:      []budget.Transaction{{ID: 1, Type: budget.TypeExpense, AmountCents: 1000, Date: date(2024, 5, 1), Category: "Keep"}},
		nextID:    1,
		deleteErr: errors.New("boom"),
	}
	h := newTestServer(fake)

	rec := do(t, h, http.MethodDelete, "/transactions/1")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500\n%s", rec.Code, rec.Body.String())
	}
	if len(fake.txns) != 1 {
		t.Errorf("target transaction must be preserved on store failure, got %d", len(fake.txns))
	}
}
