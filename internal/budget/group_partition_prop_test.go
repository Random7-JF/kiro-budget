package budget

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// genGroupTxns builds a random slice of transactions whose dates span a wide
// range (multiple days, weeks, and months) so that grouping into any period
// produces multiple buckets. Types, amounts, and categories are randomized but
// are irrelevant to the partition property; only the dates and identities
// matter here.
func genGroupTxns(t *rapid.T) []Transaction {
	categoryPool := []string{"Groceries", "Rent", "Salary", "Gifts", "Utilities", "Dining"}
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	n := rapid.IntRange(0, 40).Draw(t, "n")
	txns := make([]Transaction, 0, n)
	for i := 0; i < n; i++ {
		typ := TypeExpense
		if rapid.Bool().Draw(t, "isIncome") {
			typ = TypeIncome
		}
		// Offset up to ~5.5 years to guarantee spanning many days/weeks/months.
		dayOffset := rapid.IntRange(0, 2000).Draw(t, "dayOffset")
		date := base.AddDate(0, 0, dayOffset)

		txns = append(txns, Transaction{
			ID:          int64(i + 1),
			Type:        typ,
			AmountCents: rapid.Int64Range(1, 99_999_999_999).Draw(t, "cents"),
			Date:        date,
			Category:    rapid.SampledFrom(categoryPool).Draw(t, "category"),
		})
	}
	return txns
}

// expectedBucketKey computes the bucket key a transaction date belongs to under
// the given period, independently of the implementation under test.
func expectedBucketKey(date time.Time, period TimePeriod) string {
	switch period {
	case PeriodDay:
		return date.Format("2006-01-02")
	case PeriodWeek:
		isoYear, isoWeek := date.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", isoYear, isoWeek)
	default: // month and any unrecognized period default to month
		return date.Format("2006-01")
	}
}

// Feature: budget-tracker, Property 11: Grouping is an exhaustive, disjoint, ordered partition
func TestGroupPartitionProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		period := rapid.SampledFrom([]TimePeriod{PeriodDay, PeriodWeek, PeriodMonth}).Draw(t, "period")
		txns := genGroupTxns(t)

		groups := GroupByPeriod(txns, period)

		// (1) Every transaction appears in exactly one group, in the bucket that
		//     matches its date. We tally each transaction id across all groups and
		//     assert its containing group's Key equals the expected bucket key.
		seen := make(map[int64]int)
		for _, g := range groups {
			for _, item := range g.Items {
				seen[item.ID]++
				wantKey := expectedBucketKey(item.Date, period)
				if g.Key != wantKey {
					t.Fatalf("transaction id=%d date=%s placed in group %q, want bucket %q (period %q)",
						item.ID, item.Date.Format("2006-01-02"), g.Key, wantKey, period)
				}
			}
		}

		// (2) The union (multiset) of all groups' Items equals the input set:
		//     same size, and every input transaction appears exactly once.
		var totalItems int
		for _, g := range groups {
			totalItems += len(g.Items)
		}
		if totalItems != len(txns) {
			t.Fatalf("union of group items has %d transactions, want %d (period %q)",
				totalItems, len(txns), period)
		}
		for _, tr := range txns {
			if seen[tr.ID] != 1 {
				t.Fatalf("transaction id=%d appears %d times across groups, want exactly 1 (period %q)",
					tr.ID, seen[tr.ID], period)
			}
		}

		// (3) Group keys are ordered strictly descending (most recent first).
		//     Strictness also confirms each bucket key is distinct (disjoint buckets).
		for i := 1; i < len(groups); i++ {
			if !(groups[i-1].Key > groups[i].Key) {
				t.Fatalf("groups not strictly descending by key: %q then %q (period %q)",
					groups[i-1].Key, groups[i].Key, period)
			}
		}
	})
}
