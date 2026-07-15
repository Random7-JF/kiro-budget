package budget

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGroupByPeriod_Day(t *testing.T) {
	txns := []Transaction{
		{ID: 1, Type: TypeIncome, AmountCents: 1000, Date: d("2024-05-13"), Category: "Salary"},
		{ID: 2, Type: TypeExpense, AmountCents: 300, Date: d("2024-05-13"), Category: "Food"},
		{ID: 3, Type: TypeExpense, AmountCents: 200, Date: d("2024-05-15"), Category: "Gas"},
	}

	groups := GroupByPeriod(txns, PeriodDay)
	if len(groups) != 2 {
		t.Fatalf("expected 2 day groups, got %d", len(groups))
	}
	// Most-recent-first ordering.
	if groups[0].Key != "2024-05-15" || groups[1].Key != "2024-05-13" {
		t.Fatalf("unexpected order: %q, %q", groups[0].Key, groups[1].Key)
	}
	// Group summary for 2024-05-13: income 1000, expense 300, net 700.
	g := groups[1]
	if g.Summary.TotalIncomeCents != 1000 || g.Summary.TotalExpenseCents != 300 || g.Summary.NetBalanceCents != 700 {
		t.Fatalf("unexpected summary for %s: %+v", g.Key, g.Summary)
	}
	if len(g.Items) != 2 {
		t.Fatalf("expected 2 items in %s, got %d", g.Key, len(g.Items))
	}
}

func TestGroupByPeriod_Week_MondayStart(t *testing.T) {
	// 2024-05-13 is a Monday; 2024-05-19 is the Sunday of the same ISO week.
	// 2024-05-20 is the next Monday (new week).
	txns := []Transaction{
		{ID: 1, Type: TypeExpense, AmountCents: 100, Date: d("2024-05-13"), Category: "A"},
		{ID: 2, Type: TypeExpense, AmountCents: 100, Date: d("2024-05-19"), Category: "A"},
		{ID: 3, Type: TypeExpense, AmountCents: 100, Date: d("2024-05-20"), Category: "A"},
	}
	groups := GroupByPeriod(txns, PeriodWeek)
	if len(groups) != 2 {
		t.Fatalf("expected 2 week groups, got %d", len(groups))
	}
	// Most recent week first.
	if groups[0].Key != "2024-W21" || groups[1].Key != "2024-W20" {
		t.Fatalf("unexpected week keys/order: %q, %q", groups[0].Key, groups[1].Key)
	}
	// The Mon-Sun week (W20) should hold both the Monday and Sunday txns.
	if len(groups[1].Items) != 2 {
		t.Fatalf("expected 2 items in W20, got %d", len(groups[1].Items))
	}
}

func TestGroupByPeriod_Month(t *testing.T) {
	txns := []Transaction{
		{ID: 1, Type: TypeExpense, AmountCents: 100, Date: d("2024-04-30"), Category: "A"},
		{ID: 2, Type: TypeExpense, AmountCents: 100, Date: d("2024-05-01"), Category: "A"},
	}
	groups := GroupByPeriod(txns, PeriodMonth)
	if len(groups) != 2 {
		t.Fatalf("expected 2 month groups, got %d", len(groups))
	}
	if groups[0].Key != "2024-05" || groups[1].Key != "2024-04" {
		t.Fatalf("unexpected month keys/order: %q, %q", groups[0].Key, groups[1].Key)
	}
	if groups[0].Label != "May 2024" {
		t.Fatalf("unexpected month label: %q", groups[0].Label)
	}
}

func TestGroupByPeriod_Empty(t *testing.T) {
	groups := GroupByPeriod(nil, PeriodMonth)
	if groups == nil || len(groups) != 0 {
		t.Fatalf("expected empty non-nil slice, got %#v", groups)
	}
}

func TestGroupByPeriod_UnknownDefaultsToMonth(t *testing.T) {
	txns := []Transaction{
		{ID: 1, Type: TypeExpense, AmountCents: 100, Date: d("2024-05-01"), Category: "A"},
	}
	groups := GroupByPeriod(txns, TimePeriod("bogus"))
	if len(groups) != 1 || groups[0].Key != "2024-05" {
		t.Fatalf("expected month bucketing for unknown period, got %#v", groups)
	}
}
