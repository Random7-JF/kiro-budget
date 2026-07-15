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

// handleCreate creates a transaction for the signed-in user.
//
//   - Validation errors -> HTTP 422 with inline field messages.
//   - Store failure -> HTTP 500, stored data unchanged.
//   - Success -> HTTP 200 with re-rendered Dashboard + Log fragments.
func (s *Server) handleCreate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		s.renderError(w, stdhttp.StatusBadRequest, "The submitted form could not be read.")
		return
	}

	tx, errs := budget.Validate(formInput(r, 0))
	if len(errs) > 0 {
		s.renderValidationErrors(w, errs)
		return
	}

	if _, err := st.CreateTransaction(r.Context(), tx); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The transaction could not be saved. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// handleUpdate edits a transaction owned by the signed-in user.
func (s *Server) handleUpdate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
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

	if _, err := st.UpdateTransaction(r.Context(), tx); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderNotFound(w, "The requested transaction was not found.")
			return
		}
		s.renderError(w, stdhttp.StatusInternalServerError, "The transaction could not be updated. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// handleDelete removes a transaction owned by the signed-in user.
func (s *Server) handleDelete(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	st := storeFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.renderNotFound(w, "The requested transaction was not found.")
		return
	}

	if err := st.DeleteTransaction(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.renderNotFound(w, "The requested transaction was not found.")
			return
		}
		s.renderError(w, stdhttp.StatusInternalServerError, "The transaction could not be deleted. Please try again.")
		return
	}

	s.renderLogAndDashboard(w, r, stdhttp.StatusOK)
}

// formInput binds the submitted form fields into a TransactionInput.
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

// parseTxType maps the "type" form value to a TxType, defaulting to expense.
func parseTxType(s string) budget.TxType {
	if s == string(budget.TypeIncome) {
		return budget.TypeIncome
	}
	return budget.TypeExpense
}

// renderLogAndDashboard writes the combined Dashboard + Log fragment (plus an
// out-of-band category datalist) scoped to the viewed month. Shared success
// response for create, edit, and delete (Requirement 10.2).
func (s *Server) renderLogAndDashboard(w stdhttp.ResponseWriter, r *stdhttp.Request, status int) {
	st := storeFrom(r.Context())
	params := parseViewParams(r)

	txns, err := st.ListTransactions(r.Context())
	if err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}

	month := resolveMonth(r.FormValue("month"), txns)
	monthTxns := budget.FilterMonth(txns, month)
	log := budget.Sort(budget.Filter(monthTxns, params.TypeFilter), params.SortField, params.SortDir)
	dashboard := buildDashboard(r.Context(), st, month, monthTxns)
	logView := web.LogView{Grouped: true, Groups: budget.GroupByPeriod(log, params.Period)}

	var buf bytes.Buffer
	if err := web.Render(&buf, "dashboard", dashboard); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}
	if err := web.Render(&buf, "log", logView); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}
	if err := web.Render(&buf, "categories", categoryOptions(txns)); err != nil {
		s.renderError(w, stdhttp.StatusInternalServerError, "The page could not be loaded. Please try again.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// renderValidationErrors writes an HTTP 422 response listing per-field
// validation messages (HTML-escaped).
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
