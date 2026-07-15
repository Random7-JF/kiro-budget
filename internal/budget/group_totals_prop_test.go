package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genGroupTotalsTxn builds a random transaction with a random type, a random
// positive cents amount, and a date spread across a range wide enough to land
// in several distinct day/week/month buckets. Uniquely named to avoid clashing
// with helpers declared in other test files.
func genGroupTotalsTxn(t *rapid.T, id int64) Transaction {
	typ := TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = TypeIncome
	}
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, "cents")
	// Spread dates across ~120 days so day, week, and month buckets all vary.
	dayOffset := rapid.IntRange(0, 120).Draw(t, "dayOffset")
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return Transaction{
		ID:          id,
		Type:        typ,
		AmountCents: cents,
		Date:        base.AddDate(0, 0, dayOffset),
		Category:    rapid.SampledFrom([]string{"A", "B", "C", "D"}).Draw(t, "category"),
	}
}

// Feature: budget-tracker, Property 12: Per-group totals are consistent with overall totals
func TestGroupTotalsConsistencyProperty(t *testing.T) {
	periods := []TimePeriod{PeriodDay, PeriodWeek, PeriodMonth}

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 60).Draw(t, "n")
		txns := make([]Transaction, n)
		for i := 0; i < n; i++ {
			txns[i] = genGroupTotalsTxn(t, int64(i+1))
		}

		// Independently compute the overall income/expense totals.
		var overallIncome, overallExpense int64
		for _, tr := range txns {
			switch tr.Type {
			case TypeIncome:
				overallIncome += tr.AmountCents
			case TypeExpense:
				overallExpense += tr.AmountCents
			}
		}

		for _, period := range periods {
			groups := GroupByPeriod(txns, period)

			var sumIncome, sumExpense int64
			for _, g := range groups {
				// Each group's net balance must equal its own income minus expense.
				if g.Summary.NetBalanceCents != g.Summary.TotalIncomeCents-g.Summary.TotalExpenseCents {
					t.Fatalf("period %s group %q: net = %d, want income(%d) - expense(%d) = %d",
						period, g.Key, g.Summary.NetBalanceCents,
						g.Summary.TotalIncomeCents, g.Summary.TotalExpenseCents,
						g.Summary.TotalIncomeCents-g.Summary.TotalExpenseCents)
				}
				sumIncome += g.Summary.TotalIncomeCents
				sumExpense += g.Summary.TotalExpenseCents
			}

			// The sums over all groups must equal the overall totals.
			if sumIncome != overallIncome {
				t.Fatalf("period %s: sum of group income = %d, want overall income %d",
					period, sumIncome, overallIncome)
			}
			if sumExpense != overallExpense {
				t.Fatalf("period %s: sum of group expense = %d, want overall expense %d",
					period, sumExpense, overallExpense)
			}
		}
	})
}
