package budget

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genSortOrderTxn builds a random transaction for exercising Sort. Dates and
// amounts are drawn from deliberately small pools so duplicate dates and
// duplicate amounts occur frequently, exercising tie-breaking. IDs are assigned
// by the caller and are distinct within a generated set.
func genSortOrderTxn(t *rapid.T, id int64) Transaction {
	// Small pool of dates so ties on date are common.
	dateOffset := rapid.IntRange(0, 4).Draw(t, "dateOffset")
	base := time.Date(2024, 5, 10, 0, 0, 0, 0, time.UTC)
	date := base.AddDate(0, 0, dateOffset)

	// Small pool of amounts so ties on amount are common.
	cents := rapid.Int64Range(100, 105).Draw(t, "cents")

	typ := TypeExpense
	if rapid.Bool().Draw(t, "isIncome") {
		typ = TypeIncome
	}

	return Transaction{
		ID:          id,
		Type:        typ,
		AmountCents: cents,
		Date:        date,
		Category:    "C",
	}
}

// genSortOrderTxns generates a slice of transactions with distinct, shuffled
// IDs so that input order does not trivially match ID order.
func genSortOrderTxns(t *rapid.T) []Transaction {
	n := rapid.IntRange(0, 30).Draw(t, "n")

	// Distinct IDs 1..n, then permuted by drawing from a shrinking pool so
	// input order is generally not the same as id order (exercising that
	// tie-breaking reorders by id rather than preserving input order).
	pool := make([]int64, n)
	for i := 0; i < n; i++ {
		pool[i] = int64(i + 1)
	}
	perm := make([]int64, 0, n)
	for len(pool) > 0 {
		idx := rapid.IntRange(0, len(pool)-1).Draw(t, "pickIdx")
		perm = append(perm, pool[idx])
		pool = append(pool[:idx], pool[idx+1:]...)
	}

	txns := make([]Transaction, n)
	for i := 0; i < n; i++ {
		txns[i] = genSortOrderTxn(t, perm[i])
	}
	return txns
}

// Feature: budget-tracker, Property 13: Sorting orders correctly with deterministic tie-breaking
func TestSortOrderingProperty(t *testing.T) {
	// fieldValue returns the comparable value of the selected sort field.
	// It returns (dateUnix, amount) and the caller uses the relevant one.
	rapid.Check(t, func(t *rapid.T) {
		txns := genSortOrderTxns(t)

		// Recognized (field, dir) combinations.
		type combo struct {
			field SortField
			dir   SortDir
		}
		combos := []combo{
			{SortByDate, SortAsc},
			{SortByDate, SortDesc},
			{SortByAmount, SortAsc},
			{SortByAmount, SortDesc},
		}

		for _, c := range combos {
			got := Sort(txns, c.field, c.dir)

			if len(got) != len(txns) {
				t.Fatalf("field=%s dir=%s: length changed from %d to %d",
					c.field, c.dir, len(txns), len(got))
			}

			for i := 0; i+1 < len(got); i++ {
				a, b := got[i], got[i+1]
				cmp := compareOnField(a, b, c.field)

				switch {
				case cmp == 0:
					// Tie on the sort field: must be ordered by ascending ID.
					if a.ID >= b.ID {
						t.Fatalf("field=%s dir=%s: tie not broken by ascending id at %d: id %d then %d",
							c.field, c.dir, i, a.ID, b.ID)
					}
				case c.dir == SortAsc:
					if cmp > 0 {
						t.Fatalf("field=%s dir=asc: out of order at %d (a > b)", c.field, i)
					}
				default: // SortDesc
					if cmp < 0 {
						t.Fatalf("field=%s dir=desc: out of order at %d (a < b)", c.field, i)
					}
				}
			}
		}

		// Unrecognized field and/or direction must yield the default
		// ordering: Transaction_Date descending (ties by ascending id).
		defaultCases := []combo{
			{SortField("bogus"), SortAsc},
			{SortByDate, SortDir("sideways")},
			{SortField("nope"), SortDir("nope")},
			{SortField(""), SortDir("")},
		}
		want := Sort(txns, SortByDate, SortDesc)
		for _, c := range defaultCases {
			got := Sort(txns, c.field, c.dir)
			if len(got) != len(want) {
				t.Fatalf("default case field=%q dir=%q: length %d, want %d",
					c.field, c.dir, len(got), len(want))
			}
			for i := range got {
				if got[i].ID != want[i].ID {
					t.Fatalf("default case field=%q dir=%q: at %d got id %d, want id %d (date-desc default)",
						c.field, c.dir, i, got[i].ID, want[i].ID)
				}
			}
		}
	})
}

// compareOnField returns -1, 0, or +1 comparing a and b on the given sort
// field. It mirrors the ordering semantics of Sort (date or amount).
func compareOnField(a, b Transaction, field SortField) int {
	switch field {
	case SortByAmount:
		switch {
		case a.AmountCents < b.AmountCents:
			return -1
		case a.AmountCents > b.AmountCents:
			return 1
		default:
			return 0
		}
	default: // SortByDate
		switch {
		case a.Date.Before(b.Date):
			return -1
		case a.Date.After(b.Date):
			return 1
		default:
			return 0
		}
	}
}
