package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/budget-tracker/budget-tracker/internal/auth"
	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// doWithCookie issues a request carrying the given session cookie.
func doWithCookie(t *testing.T, h http.Handler, method, target string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// login posts credentials and returns the session cookie from the response.
func login(t *testing.T, h http.Handler, username, password string) *http.Cookie {
	t.Helper()
	rec := doForm(t, h, http.MethodPost, "/login", url.Values{
		"username": {username}, "password": {password},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303\n%s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatalf("login did not set a session cookie")
	return nil
}

func TestAuthUnauthenticatedRedirectsToLogin(t *testing.T) {
	repo := itNewRepo(t)
	h := NewServer(repo).Routes()

	for _, target := range []string{"/", "/transactions", "/dashboard"} {
		rec := do(t, h, http.MethodGet, target)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("GET %s status = %d, want 303 redirect", target, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/login" {
			t.Errorf("GET %s redirected to %q, want /login", target, loc)
		}
	}
}

func TestAuthLoginSuccessGrantsAccess(t *testing.T) {
	repo := itNewRepo(t)
	if _, err := auth.EnsureDemoUser(context.Background(), repo); err != nil {
		t.Fatalf("EnsureDemoUser: %v", err)
	}
	h := NewServer(repo).Routes()

	cookie := login(t, h, auth.DemoUsername, auth.DemoPassword)

	rec := doWithCookie(t, h, http.MethodGet, "/", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("expected full page for authenticated user")
	}
	if !strings.Contains(rec.Body.String(), auth.DemoUsername) {
		t.Errorf("expected username in header")
	}
}

func TestAuthLoginBadPassword(t *testing.T) {
	repo := itNewRepo(t)
	if _, err := auth.EnsureDemoUser(context.Background(), repo); err != nil {
		t.Fatalf("EnsureDemoUser: %v", err)
	}
	h := NewServer(repo).Routes()

	rec := doForm(t, h, http.MethodPost, "/login", url.Values{
		"username": {auth.DemoUsername}, "password": {"wrong"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad-password login status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Invalid username or password") {
		t.Errorf("expected invalid-credentials message")
	}
}

func TestAuthLogoutClearsSession(t *testing.T) {
	repo := itNewRepo(t)
	if _, err := auth.EnsureDemoUser(context.Background(), repo); err != nil {
		t.Fatalf("EnsureDemoUser: %v", err)
	}
	h := NewServer(repo).Routes()

	cookie := login(t, h, auth.DemoUsername, auth.DemoPassword)

	rec := doWithCookie(t, h, http.MethodPost, "/logout", cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want 303", rec.Code)
	}

	// The old session cookie should no longer grant access.
	rec = doWithCookie(t, h, http.MethodGet, "/", cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("after logout, GET / status = %d loc = %q, want redirect to /login", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAuthDataIsolationBetweenUsers(t *testing.T) {
	repo := itNewRepo(t)
	ctx := context.Background()
	hash, _ := auth.HashPassword("pw")
	alice, err := repo.EnsureUser(ctx, "alice", hash)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := repo.EnsureUser(ctx, "bob", hash)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	if _, err := repo.CreateTransaction(ctx, alice.ID,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 1000, Date: date(2026, 7, 1), Category: "AliceGroceries"}); err != nil {
		t.Fatalf("alice txn: %v", err)
	}
	if _, err := repo.CreateTransaction(ctx, bob.ID,
		budget.Transaction{Type: budget.TypeExpense, AmountCents: 2000, Date: date(2026, 7, 1), Category: "BobRent"}); err != nil {
		t.Fatalf("bob txn: %v", err)
	}

	h := NewServer(repo).Routes()

	aliceView := doWithCookie(t, h, http.MethodGet, "/?month=2026-07", login(t, h, "alice", "pw")).Body.String()
	if !strings.Contains(aliceView, "AliceGroceries") || strings.Contains(aliceView, "BobRent") {
		t.Errorf("alice should see only her data")
	}

	bobView := doWithCookie(t, h, http.MethodGet, "/?month=2026-07", login(t, h, "bob", "pw")).Body.String()
	if !strings.Contains(bobView, "BobRent") || strings.Contains(bobView, "AliceGroceries") {
		t.Errorf("bob should see only his data")
	}
}
