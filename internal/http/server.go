package http

import (
	"bytes"
	"context"
	stdhttp "net/http"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/web"
)

// Store is the subset of data-access operations the HTTP handlers depend on.
// Defining it here (rather than depending on the concrete *store.Repo) keeps
// the handlers testable with an in-memory fake and lets the view handlers
// (this file) and the mutation handlers (task 12.2) share a single dependency.
//
// The concrete *store.Repo satisfies this interface. store.ErrNotFound is the
// not-found sentinel returned by Get/Update/DeleteTransaction.
type Store interface {
	ListTransactions(ctx context.Context) ([]budget.Transaction, error)
	CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error)
	UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error)
	DeleteTransaction(ctx context.Context, id int64) error
	GetTransaction(ctx context.Context, id int64) (budget.Transaction, error)
}

// BudgetStore is an optional capability for managing per-category monthly
// budgets. The concrete *store.Repo implements it; handlers access it via a
// type assertion so that stores without budget support degrade gracefully.
type BudgetStore interface {
	ListBudgets(ctx context.Context) ([]budget.CategoryBudget, error)
	SetBudget(ctx context.Context, category string, cents int64) error
	DeleteBudget(ctx context.Context, category string) error
}

// RecurringStore is an optional capability for managing recurring transaction
// rules and posting them for a month. The concrete *store.Repo implements it.
type RecurringStore interface {
	ListRecurring(ctx context.Context) ([]budget.RecurringRule, error)
	CreateRecurring(ctx context.Context, rule budget.RecurringRule) (budget.RecurringRule, error)
	DeleteRecurring(ctx context.Context, id int64) error
	PostRecurringForMonth(ctx context.Context, m budget.Month) (int, error)
}

// BillStore is an optional capability for managing bills (recurring
// obligations to payees). The concrete *store.Repo implements it.
type BillStore interface {
	ListBills(ctx context.Context) ([]budget.Bill, error)
	CreateBill(ctx context.Context, b budget.Bill) (budget.Bill, error)
	DeleteBill(ctx context.Context, id int64) error
}

// Server holds the dependencies shared by all HTTP handlers.
type Server struct {
	store Store
}

// NewServer builds a Server backed by the given Store.
func NewServer(store Store) *Server {
	return &Server{store: store}
}

// budgetsFor returns the configured budgets, or nil when the store does not
// support budgets or the lookup fails.
func (s *Server) budgetsFor(ctx context.Context) []budget.CategoryBudget {
	if bs, ok := s.store.(BudgetStore); ok {
		if b, err := bs.ListBudgets(ctx); err == nil {
			return b
		}
	}
	return nil
}

// recurringFor returns the configured recurring rules, or nil when the store
// does not support recurring rules or the lookup fails.
func (s *Server) recurringFor(ctx context.Context) []budget.RecurringRule {
	if rs, ok := s.store.(RecurringStore); ok {
		if r, err := rs.ListRecurring(ctx); err == nil {
			return r
		}
	}
	return nil
}

// billsFor returns the configured bills, or nil when the store does not support
// bills or the lookup fails.
func (s *Server) billsFor(ctx context.Context) []budget.Bill {
	if bs, ok := s.store.(BillStore); ok {
		if b, err := bs.ListBills(ctx); err == nil {
			return b
		}
	}
	return nil
}

// resolveMonth chooses the month to display: the "month" parameter when it is a
// valid YYYY-MM value, otherwise the month of the most recent transaction, and
// finally the current calendar month when there are no transactions.
func resolveMonth(monthParam string, txns []budget.Transaction) budget.Month {
	if m, ok := budget.ParseMonth(monthParam); ok {
		return m
	}
	return budget.LatestMonth(txns, budget.MonthOf(time.Now()))
}

// buildDashboard assembles the month-scoped dashboard view: totals, budget
// progress, and the month label. (The category chart is derived from the
// summary in the template.)
func (s *Server) buildDashboard(ctx context.Context, m budget.Month, monthTxns []budget.Transaction) web.DashboardView {
	sum := budget.Summary(monthTxns)
	return web.DashboardView{
		Summary:    sum,
		MonthLabel: m.Label(),
		Budgets:    budget.BudgetStatus(budget.SpentByCategory(sum), s.budgetsFor(ctx)),
	}
}

// Routes builds and returns the HTTP handler with all routes registered.
//
// Task 12.1 registers the GET (view) routes and the 404 catch-all. The
// mutation routes (POST /transactions, POST /transactions/{id},
// DELETE /transactions/{id}) are owned by task 12.2, which registers them by
// adding a call to its own registration helper here.
func (s *Server) Routes() stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	s.registerViewRoutes(mux)
	s.registerMutationRoutes(mux)
	s.registerConfigRoutes(mux)
	return mux
}

// registerViewRoutes registers the read-only view routes and the not-found
// fallback. "GET /{$}" matches only the exact root path, while the bare "/"
// pattern is the catch-all that returns 404 for any unmatched route
// (Requirement 10.3).
func (s *Server) registerViewRoutes(mux *stdhttp.ServeMux) {
	mux.HandleFunc("GET /{$}", s.handleRoot)
	mux.HandleFunc("GET /transactions", s.handleLog)
	mux.HandleFunc("GET /dashboard", s.handleDashboard)
	mux.HandleFunc("/", s.handleNotFound)
}

// viewParams holds the parsed query parameters shared by the view handlers.
type viewParams struct {
	Period     budget.TimePeriod
	TypeFilter budget.TypeFilter
	SortField  budget.SortField
	SortDir    budget.SortDir
}

// parseViewParams reads period/type/sort/dir from the request query, applying
// the domain parsers' defaults (period -> month, sort -> date descending) and
// mapping the type parameter to a TypeFilter.
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

// parseTypeFilter maps the "type" query parameter to a TypeFilter, defaulting
// to FilterAll (both types) for absent or unrecognized values.
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

// handleRoot serves the application root path with a complete HTML page that
// renders both the Dashboard and the Transaction Log (Requirement 10.1).
//
// If transactions cannot be loaded from the store, it renders the error page
// with HTTP 500 and no partial content, leaving stored data unchanged
// (Requirement 10.4).
func (s *Server) handleRoot(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	params := parseViewParams(r)

	txns, err := s.store.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}

	// Scope the whole page to the selected month; the dashboard, budgets,
	// chart, and log all reflect that month (Requirements 5.1, 5.2, 5.5).
	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)

	dash := s.buildDashboard(r.Context(), month, monthTxns)
	view := web.PageView{
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
		Recurring:  s.recurringFor(r.Context()),
	}

	// Bills breakdown normalized to the selected bill period (independent of
	// the month scope, since bills are ongoing obligations).
	billPeriod := budget.ParseBillPeriod(r.URL.Query().Get("billperiod"))
	view.Bills = budget.BuildBillBreakdown(s.billsFor(r.Context()), billPeriod)
	view.BillPeriod = string(billPeriod)

	s.render(w, stdhttp.StatusOK, "page", view)
}

// handleLog serves the Transaction Log fragment (no full-page shell). It
// applies the type filter, sort, and time-period grouping.
//
// If transactions cannot be retrieved, it returns an error response indicating
// the log could not be loaded with HTTP 500 and renders no partial rows
// (Requirement 6.7).
func (s *Server) handleLog(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	params := parseViewParams(r)

	txns, err := s.store.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The transaction log could not be loaded. Please try again.")
		return
	}

	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)
	view := web.LogView{
		Grouped: true,
		Groups:  budget.GroupByPeriod(log, params.Period),
	}
	s.render(w, stdhttp.StatusOK, "log", view)
}

// handleDashboard serves the Dashboard summary fragment (no full-page shell).
//
// If the totals cannot be computed because the store read fails, it renders the
// dashboard with an "unavailable" indicator rather than failing hard
// (Requirements 5.6, 7.5).
func (s *Server) handleDashboard(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	txns, err := s.store.ListTransactions(r.Context())
	if err != nil {
		s.render(w, stdhttp.StatusOK, "dashboard", web.DashboardView{TotalsUnavailable: true})
		return
	}
	month := resolveMonth(r.URL.Query().Get("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	s.render(w, stdhttp.StatusOK, "dashboard", s.buildDashboard(r.Context(), month, monthTxns))
}

// handleNotFound returns an HTTP 404 response with the not-found error page for
// any route that is not otherwise matched (Requirement 10.3).
func (s *Server) handleNotFound(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	s.renderNotFound(w, "The requested resource was not found.")
}

// errorView is the data model for the error.html and 404.html templates.
type errorView struct {
	Message string
}

// render executes a named template into a buffer and, only on success, writes
// it with the given status code. Buffering avoids emitting a partial response
// (and partial transaction rows) if rendering fails midway (Requirement 6.7).
func (s *Server) render(w stdhttp.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := web.Render(&buf, name, data); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderError renders the standalone error page with the given status and
// message. It falls back to a minimal inline message if the template fails.
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

// renderNotFound renders the standalone 404 page with HTTP 404 and the given
// message. It falls back to a minimal inline message if the template fails.
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
