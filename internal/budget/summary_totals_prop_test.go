package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genSummaryTxn builds a random transaction with a random type, a random
// positive cents amount, and a random category. The id and date are not
// relevant to totals/net-balance, so they are kept simple.
func genSummaryTxn(t *rapid.T, id int64) Transaction {
	typ := TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = TypeIncome
	}
	// Positive cents within the valid domain (1 .. 99,999,999,999).
	cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, "cents")
	category := rapid.SampledFrom([]string{
		"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining",
	}).Draw(t, "category")

	return Transaction{
		ID:          id,
		Type:        typ,
		AmountCents: cents,
		Date:        time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC),
		Category:    category,
	}
}

// Feature: budget-tracker, Property 8: Summary totals and net balance are correct
func TestSummaryTotalsProperty(t *testing.T) {
	// Empty (and nil) input must yield zero totals and zero net balance.
	empty := Summary(nil)
	if empty.TotalIncomeCents != 0 || empty.TotalExpenseCents != 0 || empty.NetBalanceCents != 0 {
		t.Fatalf("empty input: expected all-zero totals, got %+v", empty)
	}

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 50).Draw(t, "n")
		txns := make([]Transaction, n)
		for i := 0; i < n; i++ {
			txns[i] = genSummaryTxn(t, int64(i+1))
		}

		// Independently sum expected income and expense.
		var wantIncome, wantExpense int64
		for _, tr := range txns {
			switch tr.Type {
			case TypeIncome:
				wantIncome += tr.AmountCents
			case TypeExpense:
				wantExpense += tr.AmountCents
			}
		}

		got := Summary(txns)

		if got.TotalIncomeCents != wantIncome {
			t.Fatalf("total income = %d, want %d", got.TotalIncomeCents, wantIncome)
		}
		if got.TotalExpenseCents != wantExpense {
			t.Fatalf("total expense = %d, want %d", got.TotalExpenseCents, wantExpense)
		}
		if got.NetBalanceCents != wantIncome-wantExpense {
			t.Fatalf("net balance = %d, want %d (income - expense)", got.NetBalanceCents, wantIncome-wantExpense)
		}

		// When there are no transactions, totals must be zero.
		if n == 0 && (got.TotalIncomeCents != 0 || got.TotalExpenseCents != 0 || got.NetBalanceCents != 0) {
			t.Fatalf("empty generated set: expected zero totals, got %+v", got)
		}
	})
}
