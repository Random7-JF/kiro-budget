package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genSortPermTxns builds a random slice of transactions with mixed types,
// amounts, dates, and categories. Dates span several days/weeks/months and
// amounts collide frequently so that sorting must exercise its tie-breaking
// path while still preserving the full set.
func genSortPermTxns(t *rapid.T) []Transaction {
	categoryPool := []string{"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining"}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	n := rapid.IntRange(0, 30).Draw(t, "n")
	txns := make([]Transaction, 0, n)
	for i := 0; i < n; i++ {
		typ := TypeExpense
		if rapid.Bool().Draw(t, "isIncome") {
			typ = TypeIncome
		}
		// Small amount range forces frequent ties on the amount field.
		cents := rapid.Int64Range(1, 500).Draw(t, "cents")
		// Small day offset forces frequent ties on the date field and spans
		// multiple weeks/months.
		dayOffset := rapid.IntRange(0, 120).Draw(t, "dayOffset")
		category := rapid.SampledFrom(categoryPool).Draw(t, "category")

		txns = append(txns, Transaction{
			ID:          int64(i + 1),
			Type:        typ,
			AmountCents: cents,
			Date:        base.AddDate(0, 0, dayOffset),
			Category:    category,
			Description: rapid.SampledFrom([]string{"", "note", "memo"}).Draw(t, "desc"),
		})
	}
	return txns
}

// multiset builds a frequency map keyed by the fully-identifying contents of a
// transaction, so equal counts prove the two slices are permutations of one
// another with no field altered.
func sortPermMultiset(txns []Transaction) map[Transaction]int {
	m := make(map[Transaction]int, len(txns))
	for _, tr := range txns {
		m[tr]++
	}
	return m
}

// Feature: budget-tracker, Property 14: Sorting preserves the filtered result set
func TestSortPermutationProperty(t *testing.T) {
	fields := []SortField{SortByDate, SortByAmount}
	dirs := []SortDir{SortAsc, SortDesc}
	filters := []TypeFilter{FilterAll, FilterExpense, FilterIncome}

	rapid.Check(t, func(t *rapid.T) {
		txns := genSortPermTxns(t)
		field := rapid.SampledFrom(fields).Draw(t, "field")
		dir := rapid.SampledFrom(dirs).Draw(t, "dir")
		typeFilter := rapid.SampledFrom(filters).Draw(t, "filter")

		// Apply the active type filter, then sort the filtered result.
		filtered := Filter(txns, typeFilter)

		// Snapshot the filtered input to detect any mutation by Sort.
		before := make([]Transaction, len(filtered))
		copy(before, filtered)

		sorted := Sort(filtered, field, dir)

		// The sorted output must be a permutation (same multiset) of its input:
		// nothing added, removed, or altered.
		if len(sorted) != len(filtered) {
			t.Fatalf("sorted length = %d, want %d (field=%q dir=%q filter=%q)",
				len(sorted), len(filtered), field, dir, typeFilter)
		}

		wantMS := sortPermMultiset(filtered)
		gotMS := sortPermMultiset(sorted)
		if len(gotMS) != len(wantMS) {
			t.Fatalf("distinct-element count = %d, want %d (field=%q dir=%q filter=%q)",
				len(gotMS), len(wantMS), field, dir, typeFilter)
		}
		for tr, wantCount := range wantMS {
			if gotMS[tr] != wantCount {
				t.Fatalf("transaction %+v appears %d times in sorted output, want %d (field=%q dir=%q filter=%q)",
					tr, gotMS[tr], wantCount, field, dir, typeFilter)
			}
		}

		// Sort must not mutate the input slice it was given.
		for i := range before {
			if filtered[i] != before[i] {
				t.Fatalf("Sort mutated its input at index %d: got %+v, want %+v",
					i, filtered[i], before[i])
			}
		}
	})
}
