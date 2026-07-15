package web

import (
	"bytes"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/budget-tracker/budget-tracker/internal/budget"
	"pgregory.net/rapid"
)

// logRenderCategories is a fixed set of category labels used by the Property 16
// generator. It deliberately mixes plain labels with values containing
// HTML-special characters so the test exercises html/template auto-escaping.
var logRenderCategories = []string{
	"Groceries",
	"Salary",
	"Rent & Utilities",
	"<script>alert(1)</script>",
	"A&B \"quoted\"",
	"Caf\u00e9",
}

// logRenderDescriptions is a fixed set of descriptions including the empty
// string (transaction with no description) and values with HTML-special
// characters.
var logRenderDescriptions = []string{
	"",
	"May pay",
	"weekly <b>shop</b>",
	"note & memo \"x\"",
	"tip > 10%",
}

// genLogRenderTxn draws a single random transaction spanning both types, a wide
// amount range, dates at UTC midnight, and categories/descriptions drawn from
// the HTML-special-char-inclusive sets above.
func genLogRenderTxn(t *rapid.T, id int64) budget.Transaction {
	typ := budget.TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = budget.TypeIncome
	}

	cents := rapid.Int64Range(1, 99999999999).Draw(t, "cents")

	// Dates at UTC midnight across a wide range of days.
	dayOffset := rapid.IntRange(0, 4000).Draw(t, "dayOffset")
	base := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	txDate := base.AddDate(0, 0, dayOffset)

	category := rapid.SampledFrom(logRenderCategories).Draw(t, "category")
	description := rapid.SampledFrom(logRenderDescriptions).Draw(t, "description")

	return budget.Transaction{
		ID:          id,
		Type:        typ,
		AmountCents: cents,
		Date:        txDate,
		Category:    category,
		Description: description,
	}
}

// Feature: budget-tracker, Property 16: Log rendering includes all transaction fields
func TestLogRenderingProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 8).Draw(t, "n")
		txns := make([]budget.Transaction, 0, n)
		for i := 0; i < n; i++ {
			txns = append(txns, genLogRenderTxn(t, int64(i+1)))
		}

		var buf bytes.Buffer
		if err := Render(&buf, "log", LogView{Items: txns}); err != nil {
			t.Fatalf("render log: %v", err)
		}
		out := buf.String()

		for _, tx := range txns {
			// Amount formatted to exactly two decimals via the domain formatter.
			wantAmount := budget.FormatAmount(tx.AmountCents)
			if !strings.Contains(out, wantAmount) {
				t.Fatalf("rendered log missing amount %q for %+v\n%s", wantAmount, tx, out)
			}

			// Transaction_Date in ISO 8601 YYYY-MM-DD format.
			wantDate := tx.Date.Format("2006-01-02")
			if !strings.Contains(out, wantDate) {
				t.Fatalf("rendered log missing date %q for %+v\n%s", wantDate, tx, out)
			}

			// Type string ("expense" or "income").
			wantType := string(tx.Type)
			if !strings.Contains(out, wantType) {
				t.Fatalf("rendered log missing type %q for %+v\n%s", wantType, tx, out)
			}

			// Category, compared against its HTML-escaped form.
			wantCategory := html.EscapeString(tx.Category)
			if !strings.Contains(out, wantCategory) {
				t.Fatalf("rendered log missing category %q for %+v\n%s", wantCategory, tx, out)
			}

			// Description (empty when none). A non-empty description must appear
			// HTML-escaped; an empty description still renders a row carrying the
			// other fields, which the assertions above already confirm.
			if tx.Description != "" {
				wantDesc := html.EscapeString(tx.Description)
				if !strings.Contains(out, wantDesc) {
					t.Fatalf("rendered log missing description %q for %+v\n%s", wantDesc, tx, out)
				}
			}
		}
	})
}
