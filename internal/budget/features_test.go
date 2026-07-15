package budget

import (
	"testing"
	"time"
)

func mkTx(id int64, typ TxType, cents int64, y int, m time.Month, d int, cat string) Transaction {
	return Transaction{ID: id, Type: typ, AmountCents: cents, Date: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Category: cat}
}

func TestMonthBasics(t *testing.T) {
	m := Month{Year: 2026, M: time.July}
	if m.String() != "2026-07" {
		t.Errorf("String = %q, want 2026-07", m.String())
	}
	if m.Label() != "July 2026" {
		t.Errorf("Label = %q, want July 2026", m.Label())
	}
	if m.Prev().String() != "2026-06" {
		t.Errorf("Prev = %q, want 2026-06", m.Prev().String())
	}
	if m.Next().String() != "2026-08" {
		t.Errorf("Next = %q, want 2026-08", m.Next().String())
	}
	if m.Days() != 31 {
		t.Errorf("Days = %d, want 31", m.Days())
	}
	// December wraps correctly.
	dec := Month{Year: 2026, M: time.December}
	if dec.Next().String() != "2027-01" {
		t.Errorf("Dec.Next = %q, want 2027-01", dec.Next().String())
	}
	if feb := (Month{Year: 2024, M: time.February}); feb.Days() != 29 {
		t.Errorf("Feb 2024 Days = %d, want 29 (leap year)", feb.Days())
	}
}

func TestParseMonth(t *testing.T) {
	if m, ok := ParseMonth("2026-07"); !ok || m.Year != 2026 || m.M != time.July {
		t.Errorf("ParseMonth(2026-07) = %v, %v", m, ok)
	}
	for _, bad := range []string{"", "2026", "2026-13", "julyish", "2026/07"} {
		if _, ok := ParseMonth(bad); ok {
			t.Errorf("ParseMonth(%q) unexpectedly ok", bad)
		}
	}
}

func TestFilterMonthAndLatest(t *testing.T) {
	txns := []Transaction{
		mkTx(1, TypeExpense, 100, 2026, time.June, 30, "A"),
		mkTx(2, TypeExpense, 200, 2026, time.July, 1, "A"),
		mkTx(3, TypeIncome, 300, 2026, time.July, 15, "B"),
	}
	jul := FilterMonth(txns, Month{2026, time.July})
	if len(jul) != 2 {
		t.Errorf("FilterMonth July = %d txns, want 2", len(jul))
	}
	latest := LatestMonth(txns, Month{2000, time.January})
	if latest.String() != "2026-07" {
		t.Errorf("LatestMonth = %q, want 2026-07", latest.String())
	}
	// Empty falls back.
	if fb := LatestMonth(nil, Month{2030, time.March}); fb.String() != "2030-03" {
		t.Errorf("LatestMonth empty = %q, want fallback 2030-03", fb.String())
	}
}

func TestBudgetStatus(t *testing.T) {
	spent := map[string]int64{"Groceries": 8000, "Rent": 150000}
	budgets := []CategoryBudget{
		{Category: "Groceries", LimitCents: 10000},
		{Category: "Rent", LimitCents: 135000},
		{Category: "Dining", LimitCents: 5000}, // no spend
	}
	got := BudgetStatus(spent, budgets)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	// Sorted by category: Dining, Groceries, Rent.
	if got[0].Category != "Dining" || got[1].Category != "Groceries" || got[2].Category != "Rent" {
		t.Errorf("unexpected order: %+v", got)
	}
	// Groceries: 8000/10000 = 80% -> Near, not Over.
	gro := got[1]
	if gro.Percent() != 80 || !gro.Near() || gro.Over() {
		t.Errorf("Groceries progress wrong: pct=%d near=%v over=%v", gro.Percent(), gro.Near(), gro.Over())
	}
	if gro.RemainingCents() != 2000 {
		t.Errorf("Groceries remaining = %d, want 2000", gro.RemainingCents())
	}
	// Rent: 150000/135000 -> Over, BarPercent clamps to 100.
	rent := got[2]
	if !rent.Over() || rent.BarPercent() != 100 {
		t.Errorf("Rent should be over with bar clamped: over=%v bar=%d", rent.Over(), rent.BarPercent())
	}
	if rent.RemainingCents() != -15000 {
		t.Errorf("Rent remaining = %d, want -15000", rent.RemainingCents())
	}
	// Dining: no spend -> 0%.
	if got[0].Percent() != 0 {
		t.Errorf("Dining pct = %d, want 0", got[0].Percent())
	}
}

func TestCategoryChart(t *testing.T) {
	s := Summary([]Transaction{
		mkTx(1, TypeExpense, 1000, 2026, time.July, 1, "Small"),
		mkTx(2, TypeExpense, 4000, 2026, time.July, 2, "Big"),
		mkTx(3, TypeIncome, 9999, 2026, time.July, 3, "Salary"), // income excluded
	})
	bars := CategoryChart(s)
	if len(bars) != 2 {
		t.Fatalf("got %d bars, want 2 (expenses only)", len(bars))
	}
	// Ordered largest first; the largest gets 100%.
	if bars[0].Category != "Big" || bars[0].Percent != 100 {
		t.Errorf("first bar = %+v, want Big at 100%%", bars[0])
	}
	if bars[1].Category != "Small" || bars[1].Percent != 25 {
		t.Errorf("second bar = %+v, want Small at 25%%", bars[1])
	}
}
