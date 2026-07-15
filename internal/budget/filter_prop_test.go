package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genFilterTxns builds a random slice of transactions with mixed types so that
// both expense and income filtering can be exercised. IDs are assigned
// sequentially and dates are spread across a range so relative input order is
// distinguishable.
func genFilterTxns(t *rapid.T) []Transaction {
	categoryPool := []string{"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining"}

	n := rapid.IntRange(0, 30).Draw(t, "n")
	txns := make([]Transaction, 0, n)
	for i := 0; i < n; i++ {
		typ := TypeExpense
		if rapid.Bool().Draw(t, "isIncome") {
			typ = TypeIncome
		}
		cents := rapid.Int64Range(1, 99_999_999_999).Draw(t, "cents")
		category := rapid.SampledFrom(categoryPool).Draw(t, "category")
		day := rapid.IntRange(1, 28).Draw(t, "day")
		txns = append(txns, Transaction{
			ID:          int64(i + 1),
			Type:        typ,
			AmountCents: cents,
			Date:        time.Date(2024, 5, day, 0, 0, 0, 0, time.UTC),
			Category:    category,
		})
	}
	return txns
}

// sameTransactionSequence reports whether two slices contain the same
// transactions in the same order (by ID).
func sameTransactionSequence(a, b []Transaction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}

// Feature: budget-tracker, Property 15: Type filter returns only matching transactions
func TestTypeFilterProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		txns := genFilterTxns(t)

		// FilterExpense must return only expense transactions, preserving the
		// relative order of the retained transactions in the input.
		gotExpense := Filter(txns, FilterExpense)
		for _, tr := range gotExpense {
			if tr.Type != TypeExpense {
				t.Fatalf("FilterExpense returned a non-expense transaction: %+v", tr)
			}
		}
		// Verify order preservation and completeness by walking the input.
		var wantExpense []Transaction
		for _, tr := range txns {
			if tr.Type == TypeExpense {
				wantExpense = append(wantExpense, tr)
			}
		}
		if !sameTransactionSequence(gotExpense, wantExpense) {
			t.Fatalf("FilterExpense = %v, want %v (only expenses, input order)", ids(gotExpense), ids(wantExpense))
		}

		// FilterIncome must return only income transactions, in input order.
		gotIncome := Filter(txns, FilterIncome)
		for _, tr := range gotIncome {
			if tr.Type != TypeIncome {
				t.Fatalf("FilterIncome returned a non-income transaction: %+v", tr)
			}
		}
		var wantIncome []Transaction
		for _, tr := range txns {
			if tr.Type == TypeIncome {
				wantIncome = append(wantIncome, tr)
			}
		}
		if !sameTransactionSequence(gotIncome, wantIncome) {
			t.Fatalf("FilterIncome = %v, want %v (only income, input order)", ids(gotIncome), ids(wantIncome))
		}

		// Expense and income partitions together must account for every input
		// transaction exactly once.
		if len(gotExpense)+len(gotIncome) != len(txns) {
			t.Fatalf("expense (%d) + income (%d) != total (%d)", len(gotExpense), len(gotIncome), len(txns))
		}

		// FilterAll (no filter) must return all transactions, preserving the
		// original ordering.
		gotAll := Filter(txns, FilterAll)
		if !sameTransactionSequence(gotAll, txns) {
			t.Fatalf("FilterAll = %v, want %v (all transactions, original order)", ids(gotAll), ids(txns))
		}
	})
}

// ids returns the IDs of a transaction slice, for readable failure messages.
func ids(txns []Transaction) []int64 {
	out := make([]int64, len(txns))
	for i, tr := range txns {
		out[i] = tr.ID
	}
	return out
}
