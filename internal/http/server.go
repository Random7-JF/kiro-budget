package http

import (
	"bytes"
	"context"
	stdhttp "net/http"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store"
	"github.com/budget-tracker/budget-tracker/internal/web"
)

// Store is the per-user data operations the HTTP handlers depend on. The
// request-scoped value is a *store.UserScope bound to the authenticated user,
// so handlers never deal with user ids directly. Test fakes may implement only
// this subset; budget/recurring/bill capabilities are probed via type
// assertion and degrade gracefully when absent.
type Store interface {
	ListTransactions(ctx context.Context) ([]budget.Transaction, error)
	CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error)
	UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error)
	DeleteTransaction(ctx context.Context, id int64) error
	GetTransaction(ctx context.Context, id int64) (budget.Transaction, error)
}

// BudgetStore is the optional per-user budget capability.
type BudgetStore interface {
	ListBudgets(ctx context.Context) ([]budget.CategoryBudget, error)
	SetBudget(ctx context.Context, category string, cents int64) error
	DeleteBudget(ctx context.Context, category string) error
}

// RecurringStore is the optional per-user recurring-rule capability.
type RecurringStore interface {
	ListRecurring(ctx context.Context) ([]budget.RecurringRule, error)
	CreateRecurring(ctx context.Context, rule budget.RecurringRule) (budget.RecurringRule, error)
	DeleteRecurring(ctx context.Context, id int64) error
	PostRecurringForMonth(ctx context.Context, m budget.Month) (int, error)
}

// BillStore is the optional per-user bills capability.
type BillStore interface {
	ListBills(ctx context.Context) ([]budget.Bill, error)
	CreateBill(ctx context.Context, b budget.Bill) (budget.Bill, error)
	DeleteBill(ctx context.Context, id int64) error
}

// sessionCookie is the name of the session cookie.
const sessionCookie = "bt_session"

// sessionTTL is how long a login session remains valid.
const sessionTTL = 30 * 24 * time.Hour

// ctxKey is the private type for request-context keys.
type ctxKey int

const (
	storeKey ctxKey = iota
	userKey
)

func withStore(ctx context.Context, st Store) context.Context {
	return context.WithValue(ctx, storeKey, st)
}

func withUser(ctx context.Context, u store.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// storeFrom returns the request-scoped per-user store.
func storeFrom(ctx context.Context) Store {
	st, _ := ctx.Value(storeKey).(Store)
	return st
}

// userFrom returns the authenticated user for the request.
func userFrom(ctx context.Context) store.User {
	u, _ := ctx.Value(userKey).(store.User)
	return u
}

// Server holds the dependencies shared by all HTTP handlers.
type Server struct {
	repo *store.Repo
}

// NewServer builds a Server backed by the given repository.
func NewServer(repo *store.Repo) *Server {
	return &Server{repo: repo}
}

// Routes builds the HTTP handler: public auth routes plus the authenticated
// application routes and a 404 fallback.
func (s *Server) Routes() stdhttp.Handler {
	mux := s.appMux(s.requireAuth)
	mux.HandleFunc("GET /login", s.handleLoginForm)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /register", s.handleRegisterForm)
	mux.HandleFunc("POST /register", s.handleRegister)
	return mux
}

// appMux registers the application routes, wrapping each with wrap (which
// supplies the request-scoped store and user). Factored out so tests can inject
// a fixed store instead of going through session authentication.
func (s *Server) appMux(wrap func(stdhttp.HandlerFunc) stdhttp.HandlerFunc) *stdhttp.ServeMux {
	mux := stdhttp.NewServeMux()

	// Views
	mux.HandleFunc("GET /{$}", wrap(s.handleRoot))
	mux.HandleFunc("GET /transactions", wrap(s.handleLog))
	mux.HandleFunc("GET /dashboard", wrap(s.handleDashboard))

	// Mutations
	mux.HandleFunc("POST /transactions", wrap(s.handleCreate))
	mux.HandleFunc("POST /transactions/{id}", wrap(s.handleUpdate))
	mux.HandleFunc("DELETE /transactions/{id}", wrap(s.handleDelete))

	// Configuration (budgets, recurring, bills)
	mux.HandleFunc("POST /budgets", wrap(s.handleSetBudget))
	mux.HandleFunc("POST /budgets/delete", wrap(s.handleDeleteBudget))
	mux.HandleFunc("POST /recurring", wrap(s.handleCreateRecurring))
	mux.HandleFunc("POST /recurring/delete", wrap(s.handleDeleteRecurring))
	mux.HandleFunc("POST /recurring/post", wrap(s.handlePostRecurring))
	mux.HandleFunc("POST /bills", wrap(s.handleCreateBill))
	mux.HandleFunc("POST /bills/delete", wrap(s.handleDeleteBill))

	// Not-found fallback
	mux.HandleFunc("/", s.handleNotFound)
	return mux
}

// requireAuth wraps a handler so it only runs for an authenticated request. It
// resolves the session cookie to a user, binds a per-user store to the request
// context, and otherwise redirects to the login page.
func (s *Server) requireAuth(next stdhttp.HandlerFunc) stdhttp.HandlerFunc {
	return func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			stdhttp.Redirect(w, r, "/login", stdhttp.StatusSeeOther)
			return
		}
		user, err := s.repo.SessionUser(r.Context(), cookie.Value)
		if err != nil {
			clearSessionCookie(w)
			stdhttp.Redirect(w, r, "/login", stdhttp.StatusSeeOther)
			return
		}
		ctx := withUser(withStore(r.Context(), s.repo.ForUser(user.ID)), user)
		next(w, r.WithContext(ctx))
	}
}

// budgetsFor returns the configured budgets for the request store, or nil.
func budgetsFor(ctx context.Context, st Store) []budget.CategoryBudget {
	if bs, ok := st.(BudgetStore); ok {
		if b, err := bs.ListBudgets(ctx); err == nil {
			return b
		}
	}
	return nil
}

// recurringFor returns the configured recurring rules for the request store.
func recurringFor(ctx context.Context, st Store) []budget.RecurringRule {
	if rs, ok := st.(RecurringStore); ok {
		if r, err := rs.ListRecurring(ctx); err == nil {
			return r
		}
	}
	return nil
}

// billsFor returns the configured bills for the request store.
func billsFor(ctx context.Context, st Store) []budget.Bill {
	if bs, ok := st.(BillStore); ok {
		if b, err := bs.ListBills(ctx); err == nil {
			return b
		}
	}
	return nil
}

// viewParams holds the parsed query parameters shared by the view handlers.
type viewParams struct {
	Period     budget.TimePeriod
	TypeFilter budget.TypeFilter
	SortField  budget.SortField
	SortDir    budget.SortDir
}

func parseViewParams(r *stdhttp.Request) viewParams {
	q := r.URL.Query()
	field, dir := budget.ParseSort(q.Get("sort"), q.Get("dir"))
	return viewParams{
		Period:     budget.ParsePeriod(q.Get("period")),
		TypeFilter: parseTypeFilter(q.Get("type")),
		SortField:  field,
		SortDir:    dir,
	}
}

func parseTypeFilter(s string) budget.TypeFilter {
	switch s {
	case "expense":
		return budget.FilterExpense
	case "income":
		return budget.FilterIncome
	default:
		return budget.FilterAll
	}
}

// resolveMonth chooses the month to display: the "month" parameter when valid,
// otherwise the month of the most recent transaction, then the current month.
func resolveMonth(monthParam string, txns []budget.Transaction) budget.Month {
	if m, ok := budget.ParseMonth(monthParam); ok {
		return m
	}
	return budget.LatestMonth(txns, budget.MonthOf(time.Now()))
}

// buildDashboard assembles the month-scoped dashboard view.
func buildDashboard(ctx context.Context, st Store, m budget.Month, monthTxns []budget.Transaction) web.DashboardView {
	sum := budget.Summary(monthTxns)
	return web.DashboardView{
		Summary:    sum,
		MonthLabel: m.Label(),
		Budgets:    budget.BudgetStatus(budget.SpentByCategory(sum), budgetsFor(ctx, st)),
	}
}

// handleRoot serves the full month-scoped application page for the logged-in
// user (Requirements 5.1, 5.2, 5.5, 10.1).
func (s *Server) handleRoot(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
	params := parseViewParams(r)

	txns, err := st.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}

	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)

	dash := buildDashboard(r.Context(), st, month, monthTxns)
	billPeriod := budget.ParseBillPeriod(r.URL.Query().Get("billperiod"))

	view := web.PageView{
		Username:    userFrom(r.Context()).Username,
		DisplayName: userFrom(r.Context()).DisplayName(),
		Dashboard: dash,
		Log: web.LogView{
			Grouped: true,
			Groups:  budget.GroupByPeriod(log, params.Period),
		},
		Categories: categoryOptions(txns),
		Month:      month.String(),
		MonthLabel: month.Label(),
		PrevMonth:  month.Prev().String(),
		NextMonth:  month.Next().String(),
		Budgets:    dash.Budgets,
		Recurring:  recurringFor(r.Context(), st),
		Bills:      budget.BuildBillBreakdown(billsFor(r.Context(), st), billPeriod),
		BillPeriod: string(billPeriod),
	}
	s.render(w, stdhttp.StatusOK, "page", view)
}

// handleLog serves the Transaction Log fragment (Requirement 6.7).
func (s *Server) handleLog(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
	params := parseViewParams(r)

	txns, err := st.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The transaction log could not be loaded. Please try again.")
		return
	}

	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)
	view := web.LogView{Grouped: true, Groups: budget.GroupByPeriod(log, params.Period)}
	s.render(w, stdhttp.StatusOK, "log", view)
}

// handleDashboard serves the Dashboard summary fragment (Requirements 5.6, 7.5).
func (s *Server) handleDashboard(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
	txns, err := st.ListTransactions(r.Context())
	if err != nil {
		s.render(w, stdhttp.StatusOK, "dashboard", web.DashboardView{TotalsUnavailable: true})
		return
	}
	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	s.render(w, stdhttp.StatusOK, "dashboard", buildDashboard(r.Context(), st, month, monthTxns))
}

// handleNotFound returns an HTTP 404 with the not-found error page.
func (s *Server) handleNotFound(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.renderNotFound(w, "The requested resource was not found.")
}

// errorView is the data model for the error.html and 404.html templates.
type errorView struct {
	Message string
}

// render executes a named template into a buffer, then writes it with the given
// status. Buffering avoids emitting partial content if rendering fails midway.
func (s *Server) render(w stdhttp.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := web.Render(&buf, name, data); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) renderError(w stdhttp.ResponseWriter, status int, msg string) {
	var buf bytes.Buffer
	if err := web.Render(&buf, "error.html", errorView{Message: msg}); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<h1>Something went wrong</h1>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) renderNotFound(w stdhttp.ResponseWriter, msg string) {
	var buf bytes.Buffer
	if err := web.Render(&buf, "404.html", errorView{Message: msg}); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(stdhttp.StatusNotFound)
		_, _ = w.Write([]byte("<h1>404 &ndash; Not found</h1>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(stdhttp.StatusNotFound)
	_, _ = w.Write(buf.Bytes())
}
