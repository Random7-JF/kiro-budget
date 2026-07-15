package http

import (
	stdhttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// registerConfigRoutes registers the budget and recurring-rule management
// routes. These use plain form posts and redirect back to the viewed month
// (Post/Redirect/Get), so the browser lands on a fresh, fully-rendered page.
func (s *Server) registerConfigRoutes(mux *stdhttp.ServeMux) {
	mux.HandleFunc("POST /budgets", s.handleSetBudget)
	mux.HandleFunc("POST /budgets/delete", s.handleDeleteBudget)
	mux.HandleFunc("POST /recurring", s.handleCreateRecurring)
	mux.HandleFunc("POST /recurring/delete", s.handleDeleteRecurring)
	mux.HandleFunc("POST /recurring/post", s.handlePostRecurring)
	mux.HandleFunc("POST /bills", s.handleCreateBill)
	mux.HandleFunc("POST /bills/delete", s.handleDeleteBill)
}

// redirectToMonth issues a See-Other redirect back to the view the form was
// submitted from, preserving the selected month and bill-period when present.
func (s *Server) redirectToMonth(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	q := url.Values{}
	if ym := r.PostFormValue("month"); ym != "" {
		if _, ok := budget.ParseMonth(ym); ok {
			q.Set("month", ym)
		}
	}
	if bp := r.PostFormValue("billperiod"); bp != "" {
		q.Set("billperiod", bp)
	}
	target := "/"
	if enc := q.Encode(); enc != "" {
		target = "/?" + enc
	}
	stdhttp.Redirect(w, r, target, stdhttp.StatusSeeOther)
}

// handleSetBudget creates or updates a per-category monthly budget. Invalid
// input (missing category or unparseable amount) is ignored and the user is
// redirected back unchanged.
func (s *Server) handleSetBudget(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	bs, ok := s.store.(BudgetStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Budgets are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	category := strings.TrimSpace(r.PostFormValue("category"))
	cents, amountErr := budget.ParseAmount(r.PostFormValue("amount"))
	if category == "" || len([]rune(category)) > 100 || amountErr != nil {
		s.redirectToMonth(w, r)
		return
	}

	if err := bs.SetBudget(r.Context(), category, cents); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The budget could not be saved. Please try again.")
		return
	}
	s.redirectToMonth(w, r)
}

// handleDeleteBudget removes a category's budget.
func (s *Server) handleDeleteBudget(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	bs, ok := s.store.(BudgetStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Budgets are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	if category := strings.TrimSpace(r.PostFormValue("category")); category != "" {
		if err := bs.DeleteBudget(r.Context(), category); err != nil {
			s.renderError(w, stdhttp.StatusInternalServerError, "The budget could not be removed. Please try again.")
			return
		}
	}
	s.redirectToMonth(w, r)
}

// handleCreateRecurring adds a recurring transaction rule. Invalid input is
// ignored and the user is redirected back unchanged.
func (s *Server) handleCreateRecurring(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	rs, ok := s.store.(RecurringStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Recurring rules are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	cents, amountErr := budget.ParseAmount(r.PostFormValue("amount"))
	day, dayErr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("day")))
	category := strings.TrimSpace(r.PostFormValue("category"))
	if amountErr != nil || dayErr != nil || day < 1 || day > 31 || category == "" || len([]rune(category)) > 100 {
		s.redirectToMonth(w, r)
		return
	}

	rule := budget.RecurringRule{
		Type:        parseTxType(r.PostFormValue("type")),
		AmountCents: cents,
		Category:    category,
		Description: r.PostFormValue("description"),
		DayOfMonth:  day,
	}
	if _, err := rs.CreateRecurring(r.Context(), rule); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The recurring rule could not be saved. Please try again.")
		return
	}
	s.redirectToMonth(w, r)
}

// handleDeleteRecurring removes a recurring rule by id.
func (s *Server) handleDeleteRecurring(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	rs, ok := s.store.(RecurringStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Recurring rules are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	if id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64); err == nil {
		if derr := rs.DeleteRecurring(r.Context(), id); derr != nil {
			s.renderError(w, stdhttp.StatusInternalServerError, "The recurring rule could not be removed. Please try again.")
			return
		}
	}
	s.redirectToMonth(w, r)
}

// handlePostRecurring materializes all recurring rules for the submitted month
// (idempotently) and redirects back to that month.
func (s *Server) handlePostRecurring(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	rs, ok := s.store.(RecurringStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Recurring rules are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	m, ok := budget.ParseMonth(r.PostFormValue("month"))
	if !ok {
		m = budget.MonthOf(time.Now())
	}
	if _, err := rs.PostRecurringForMonth(r.Context(), m); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "Recurring transactions could not be posted. Please try again.")
		return
	}
	s.redirectToMonth(w, r)
}

// handleCreateBill adds a bill payee. Invalid input is ignored and the user is
// redirected back unchanged.
func (s *Server) handleCreateBill(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	bs, ok := s.store.(BillStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Bills are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	payee := strings.TrimSpace(r.PostFormValue("payee"))
	cents, amountErr := budget.ParseAmount(r.PostFormValue("amount"))
	freq, freqOK := budget.ParseFrequency(r.PostFormValue("frequency"))

	// Due day is optional; when provided it must be 1..31.
	due := 0
	if dueStr := strings.TrimSpace(r.PostFormValue("due_day")); dueStr != "" {
		d, err := strconv.Atoi(dueStr)
		if err != nil || d < 1 || d > 31 {
			s.redirectToMonth(w, r)
			return
		}
		due = d
	}

	if payee == "" || len([]rune(payee)) > 100 || amountErr != nil || !freqOK {
		s.redirectToMonth(w, r)
		return
	}

	bill := budget.Bill{
		Payee:       payee,
		AmountCents: cents,
		Frequency:   freq,
		Category:    strings.TrimSpace(r.PostFormValue("category")),
		DueDay:      due,
	}
	if _, err := bs.CreateBill(r.Context(), bill); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The bill could not be saved. Please try again.")
		return
	}
	s.redirectToMonth(w, r)
}

// handleDeleteBill removes a bill by id.
func (s *Server) handleDeleteBill(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	bs, ok := s.store.(BillStore)
	if !ok {
		s.renderError(w, stdhttp.StatusInternalServerError, "Bills are not supported by this data store.")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	if id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64); err == nil {
		if derr := bs.DeleteBill(r.Context(), id); derr != nil {
			s.renderError(w, stdhttp.StatusInternalServerError, "The bill could not be removed. Please try again.")
			return
		}
	}
	s.redirectToMonth(w, r)
}
