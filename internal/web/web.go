// Package web is the presentation layer for the Budget Tracker. It embeds and
// renders the html/template templates (full page shell and HTMX fragments)
// located under the templates directory.
//
// All templates use html/template, so every user-supplied value (category and
// description in particular) is contextually auto-escaped on render, which
// prevents HTML/script injection.
//
// # Template and fragment names
//
// The parsed template set exposes three named templates, defined via
// {{define ...}} blocks inside the embedded files:
//
//   - "page"      – the full HTML document shell (Requirement 10.1). Expects a
//     PageView. It embeds the dashboard and log fragments.
//   - "dashboard" – the Dashboard summary fragment. Expects a DashboardView.
//   - "log"       – the Transaction Log fragment. Expects a LogView.
//
// Handlers (see the http package) render a full page with
// Render(w, "page", PageView{...}) and HTMX fragments with
// Render(w, "dashboard", DashboardView{...}) / Render(w, "log", LogView{...}).
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

//go:embed templates/*.html
var templatesFS embed.FS

// templates is the parsed template set containing the "page", "dashboard", and
// "log" named templates plus the registered helper functions.
var templates = template.Must(
	template.New("budget").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html"),
)

// funcMap holds the template helper functions available to every template.
//
//   - money  formats an integer count of cents as a fixed two-decimal string
//     (e.g. 1250 -> "12.50", -500 -> "-5.00"), delegating to budget.FormatAmount.
//   - isodate formats a date as ISO 8601 YYYY-MM-DD (Requirement 6.1).
var funcMap = template.FuncMap{
	"money":   budget.FormatAmount,
	"isodate": func(t time.Time) string { return t.Format("2006-01-02") },
	// neg returns the negation of a cents value (used to display an
	// over-budget overage as a positive amount).
	"neg": func(c int64) int64 { return -c },
	// chart derives the spending-by-category bars from a dashboard summary so
	// the template can render them without the handler passing them explicitly.
	"chart": budget.CategoryChart,
}

// DashboardView is the view model for the Dashboard summary fragment. It wraps
// the computed summary and a flag indicating whether the totals could not be
// computed.
type DashboardView struct {
	// Summary holds the totals, net balance, and per-category breakdown.
	Summary budget.DashboardSummary
	// TotalsUnavailable is true when the totals could not be computed. When
	// set, the dashboard renders an "unavailable" indicator instead of the
	// figures (Requirements 5.6, 7.5).
	TotalsUnavailable bool
	// MonthLabel is the human-readable month the dashboard is scoped to
	// (e.g. "July 2026"). Empty when not month-scoped.
	MonthLabel string
	// Budgets holds per-category budget progress for the scoped month. The
	// spending-by-category chart is derived from Summary via the "chart"
	// template function, so it need not be passed explicitly.
	Budgets []budget.BudgetProgress
}

// LogView is the view model for the Transaction Log fragment. It supports both
// a flat listing and a grouped (time-period) listing.
type LogView struct {
	// Grouped selects grouped rendering (Groups) over the flat listing (Items).
	Grouped bool
	// Groups holds the time-period groups when Grouped is true. Each group
	// carries its own Summary and Items.
	Groups []budget.Group
	// Items holds the flat transaction listing when Grouped is false.
	Items []budget.Transaction
	// TotalsUnavailable, when set for grouped views, renders an "unavailable"
	// indicator for the per-group totals while still showing the grouped
	// transactions (Requirement 7.5).
	TotalsUnavailable bool
}

// PageView is the view model for the full-page shell. It embeds the dashboard
// and log view models rendered by the "dashboard" and "log" fragments, plus the
// category suggestions rendered by the "categories" datalist fragment.
type PageView struct {
	// Username of the signed-in user, shown in the header.
	Username   string
	Dashboard  DashboardView
	Log        LogView
	Categories []string
	// Month scoping and navigation.
	Month      string // "2026-07" for hidden fields / links
	MonthLabel string // "July 2026"
	PrevMonth  string // "2026-06"
	NextMonth  string // "2026-08"
	// Budgets are the configured per-category budgets with the scoped month's
	// spend, used by the sidebar management list.
	Budgets []budget.BudgetProgress
	// Recurring are the configured recurring transaction rules.
	Recurring []budget.RecurringRule
	// Bills is the bill breakdown normalized to the selected bill period, plus
	// the per-bill items used for the management list.
	Bills budget.BillBreakdown
	// BillPeriod is the selected bill breakdown period ("week"/"month"/"year").
	BillPeriod string
}

// LoginView is the view model for the login page.
type LoginView struct {
	// Error is an optional message shown above the form (e.g. bad credentials).
	Error string
}

// Render executes the named template ("page", "dashboard", "log", or "login")
// to w, writing the rendered HTML. All user-supplied values are auto-escaped.
func Render(w io.Writer, name string, data any) error {
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("web: render %q: %w", name, err)
	}
	return nil
}
