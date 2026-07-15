package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genCategoryTxns builds a random slice of transactions drawn from a small pool
// of categories (to force collisions and empty categories) with mixed types and
// positive amounts.
func genCategoryTxns(t *rapid.T) []Transaction {
	categoryPool := []string{"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining"}

	n := rapid.IntRange(0, 30).Draw(t, "n")
	txns := make([]Transaction, 0, n)
	for i := 0; i < n; i++ {
		typ := TypeExpense
		if rapid.Bool().Draw(t, "isIncome") {
			typ = TypeIncome
		}
		cents := rapid.Int64Range(1, 99999999999).Draw(t, "cents")
		category := rapid.SampledFrom(categoryPool).Draw(t, "category")
		txns = append(txns, Transaction{
			ID:          int64(i + 1),
			Type:        typ,
			AmountCents: cents,
			Date:        time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC),
			Category:    category,
		})
	}
	return txns
}

// Feature: budget-tracker, Property 9: Category breakdown is exact and complete
func TestSummaryCategoryProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txns := genCategoryTxns(t)

		// Independently compute the expected per-category expense totals and the
		// set of categories that have at least one expense.
		expectedTotals := make(map[string]int64)
		for _, tx := range txns {
			if tx.Type == TypeExpense {
				expectedTotals[tx.Category] += tx.AmountCents
			}
		}

		got := Summary(txns)

		// The breakdown must contain exactly one entry per expense category and
		// none for categories without an expense.
		if len(got.ByCategory) != len(expectedTotals) {
			t.Fatalf("ByCategory has %d entries, want %d (categories with >=1 expense); got=%+v want=%+v",
				len(got.ByCategory), len(expectedTotals), got.ByCategory, expectedTotals)
		}

		seen := make(map[string]bool)
		var sumOfCategoryTotals int64
		for _, ce := range got.ByCategory {
			if seen[ce.Category] {
				t.Fatalf("category %q appears more than once in ByCategory: %+v", ce.Category, got.ByCategory)
			}
			seen[ce.Category] = true

			want, ok := expectedTotals[ce.Category]
			if !ok {
				t.Fatalf("category %q has an entry but no expenses; ByCategory=%+v", ce.Category, got.ByCategory)
			}
			if ce.TotalCents != want {
				t.Fatalf("category %q total = %d, want %d", ce.Category, ce.TotalCents, want)
			}
			sumOfCategoryTotals += ce.TotalCents
		}

		// The sum of all category totals must equal the overall total expense.
		if sumOfCategoryTotals != got.TotalExpenseCents {
			t.Fatalf("sum of category totals = %d, want overall total expense %d",
				sumOfCategoryTotals, got.TotalExpenseCents)
		}
	})
}
