package http

import (
	"bytes"
	"errors"
	"html/template"
	stdhttp "net/http"
	"strconv"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"github.com/budget-tracker/budget-tracker/internal/store"
	"github.com/budget-tracker/budget-tracker/internal/web"
)

// registerMutationRoutes registers the mutating routes that create, edit, and
// delete transactions. Each handler validates input, invokes the repository,
// and (on success) returns the combined Dashboard + Transaction Log fragment so
// the HTMX client can swap in both affected regions (Requirement 10.2).
func (s *Server) registerMutationRoutes(mux *stdhttp.ServeMux) {
	mux.HandleFunc("POST /transactions", s.handleCreate)
	mux.HandleFunc("POST /transactions/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /transactions/{id}", s.handleDelete)
}

// handleCreate creates a new expense or income transaction from the submitted
// form. It binds the form fields into a TransactionInput, validates it, and
// persists it.
//
//   - Validation errors -> HTTP 422 with inline field messages, stored data
//     unchanged (Requirements 1.2-1.5, 2.2-2.5).
//   - Store failure -> HTTP 500, stored data unchanged (Requirements 9.5, 10.4).
//   - Success -> HTTP 200 with re-rendered Dashboard + Log fragments including
//     the new transaction (Requirements 1.6, 2.6, 10.2).
func (s *Server) handleCreate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	tx, errs := budget.Validate(formInput(r, 0))
	if len(errs) > 0 {
		s.renderValidationErrors(w, errs)
		return
	}

	if _, err := s.store.CreateTransaction(r.Context(), tx); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The transaction could not be saved. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// handleUpdate edits an existing transaction identified by the path id.
//
//   - Non-numeric / missing id -> HTTP 404 (there is no such transaction).
//   - Validation errors -> HTTP 422 with inline field messages, stored data
//     unchanged (Requirements 3.3, 3.4).
//   - Not-found id -> HTTP 404, stored data unchanged (Requirement 3.2).
//   - Store failure -> HTTP 500, stored data unchanged (Requirements 9.5, 10.4).
//   - Success -> HTTP 200 with re-rendered fragments reflecting the update
//     (Requirement 3.5).
func (s *Server) handleUpdate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.renderNotFound(w, "The requested transaction was not found.")
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	tx, errs := budget.Validate(formInput(r, id))
	if len(errs) > 0 {
		s.renderValidationErrors(w, errs)
		return
	}

	if _, err := s.store.UpdateTransaction(r.Context(), tx); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderNotFound(w, "The requested transaction was not found.")
			return
		}
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The transaction could not be updated. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// handleDelete removes an existing transaction identified by the path id.
//
//   - Non-numeric / missing id -> HTTP 404 (there is no such transaction).
//   - Not-found id -> HTTP 404, stored data unchanged (Requirement 4.2).
//   - Store failure -> HTTP 500, target transaction preserved (Requirement 4.4).
//   - Success -> HTTP 200 with re-rendered fragments excluding the deleted
//     transaction and retaining all others (Requirement 4.3).
func (s *Server) handleDelete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.renderNotFound(w, "The requested transaction was not found.")
		return
	}

	if err := s.store.DeleteTransaction(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderNotFound(w, "The requested transaction was not found.")
			return
		}
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The transaction could not be deleted. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// formInput binds the submitted form fields into a TransactionInput. The id is
// 0 for a create and the target transaction id for an edit. An unrecognized or
// absent "type" defaults to expense.
func formInput(r *stdhttp.Request, id int64) budget.TransactionInput {
	return budget.TransactionInput{
		ID:          id,
		Type:        parseTxType(r.PostFormValue("type")),
		AmountText:  r.PostFormValue("amount"),
		DateText:    r.PostFormValue("date"),
		Category:    r.PostFormValue("category"),
		Description: r.PostFormValue("description"),
	}
}

// parseTxType maps the submitted "type" form value to a TxType, accepting
// "expense" and "income" and defaulting to expense for anything else.
func parseTxType(s string) budget.TxType {
	if s == string(budget.TypeIncome) {
		return budget.TypeIncome
	}
	return budget.TypeExpense
}

// renderLogAndDashboard lists all transactions, applies the request's
// period/type/sort/dir selection, and writes the combined Dashboard + Log
// fragment (dashboard fragment followed by log fragment) with the given status.
// This is the shared success response for create, edit, and delete
// (Requirement 10.2): the HTMX client receives both re-rendered regions without
// a full page shell.
//
// If the store read fails while rebuilding the fragments, it renders the error
// page with HTTP 500 and no partial content.
func (s *Server) renderLogAndDashboard(w stdhttp.ResponseWriter, r *stdhttp.Request, status int) {
	params := parseViewParams(r)

	txns, err := s.store.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}

	// Scope the re-rendered fragments to the month the client is viewing (sent
	// as a "month" form field / query param), falling back to the latest month.
	month := resolveMonth(r.FormValue("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)
	dashboard := s.buildDashboard(r.Context(), month, monthTxns)
	logView := web.LogView{Grouped: true, Groups: budget.GroupByPeriod(log, params.Period)}

	// Render both fragments into a single buffer so a mid-render failure never
	// produces a partial response, then flush them together.
	var buf bytes.Buffer
	if err := web.Render(&buf, "dashboard", dashboard); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}
	if err := web.Render(&buf, "log", logView); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}
	// Refresh the category suggestions out-of-band so a newly used category
	// becomes available in the autocomplete without a full page reload.
	if err := web.Render(&buf, "categories", categoryOptions(txns)); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError,
			"The page could not be loaded. Please try again.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderValidationErrors writes an HTTP 422 response listing the per-field
// validation messages. The field name and message are HTML-escaped to prevent
// injection. This leaves stored data untouched (the caller never reached the
// store).
func (s *Server) renderValidationErrors(w stdhttp.ResponseWriter, errs []budget.FieldError) {
	var buf bytes.Buffer
	buf.WriteString(`<div class="validation-errors" role="alert"><ul>`)
	for _, fe := range errs {
		buf.WriteString(`<li class="field-error" data-field="`)
		buf.WriteString(template.HTMLEscapeString(fe.Field))
		buf.WriteString(`">`)
		buf.WriteString(template.HTMLEscapeString(fe.Message))
		buf.WriteString(`</li>`)
	}
	buf.WriteString(`</ul></div>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(stdhttp.StatusUnprocessableEntity)
	_, _ = w.Write(buf.Bytes())
}
