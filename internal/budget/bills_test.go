package budget

import "testing"

func TestFrequencyPerYearAndParse(t *testing.T) {
	cases := map[BillFrequency]int{
		FreqWeekly: 52, FreqBiweekly: 26, FreqMonthly: 12, FreqQuarterly: 4, FreqYearly: 1,
	}
	for f, want := range cases {
		if got := f.PerYear(); got != want {
			t.Errorf("%s.PerYear() = %d, want %d", f, got, want)
		}
		if parsed, ok := ParseFrequency(string(f)); !ok || parsed != f {
			t.Errorf("ParseFrequency(%q) = %q, %v", f, parsed, ok)
		}
	}
	if _, ok := ParseFrequency("fortnightly"); ok {
		t.Error("ParseFrequency(fortnightly) should not be ok")
	}
	if BillFrequency("bogus").PerYear() != 0 {
		t.Error("unknown frequency PerYear should be 0")
	}
}

func TestNormalizedCents(t *testing.T) {
	// $1200/year -> $100/month, ~$23.08/week, $1200/year.
	yearly := int64(120000)
	if got := NormalizedCents(yearly, FreqYearly, BillMonthly); got != 10000 {
		t.Errorf("yearly->monthly = %d, want 10000", got)
	}
	if got := NormalizedCents(yearly, FreqYearly, BillYearly); got != 120000 {
		t.Errorf("yearly->yearly = %d, want 120000", got)
	}
	if got := NormalizedCents(yearly, FreqYearly, BillWeekly); got != 2308 {
		t.Errorf("yearly->weekly = %d, want 2308 (rounded)", got)
	}
	// $50/month -> $600/year, $50/month.
	if got := NormalizedCents(5000, FreqMonthly, BillYearly); got != 60000 {
		t.Errorf("monthly->yearly = %d, want 60000", got)
	}
	if got := NormalizedCents(5000, FreqMonthly, BillMonthly); got != 5000 {
		t.Errorf("monthly->monthly = %d, want 5000", got)
	}
	// Weekly $20 -> $1040/year -> ~$86.67/month.
	if got := NormalizedCents(2000, FreqWeekly, BillMonthly); got != 8667 {
		t.Errorf("weekly->monthly = %d, want 8667 (rounded)", got)
	}
}

func TestBuildBillBreakdown(t *testing.T) {
	bills := []Bill{
		{ID: 1, Payee: "Netflix", AmountCents: 1599, Frequency: FreqMonthly},
		{ID: 2, Payee: "Insurance", AmountCents: 120000, Frequency: FreqYearly},
		{ID: 3, Payee: "Trash", AmountCents: 4500, Frequency: FreqQuarterly},
	}
	b := BuildBillBreakdown(bills, BillMonthly)
	if b.Period != BillMonthly {
		t.Errorf("period = %q, want month", b.Period)
	}
	// Netflix 1599 + Insurance 10000 + Trash 1500 = 13099.
	if b.TotalCents != 1599+10000+1500 {
		t.Errorf("total = %d, want %d", b.TotalCents, 1599+10000+1500)
	}
	// Sorted most-expensive first for the month: Insurance (10000), Netflix (1599), Trash (1500).
	if b.Items[0].Bill.Payee != "Insurance" || b.Items[1].Bill.Payee != "Netflix" || b.Items[2].Bill.Payee != "Trash" {
		t.Errorf("unexpected order: %s, %s, %s", b.Items[0].Bill.Payee, b.Items[1].Bill.Payee, b.Items[2].Bill.Payee)
	}
}

func TestParseBillPeriod(t *testing.T) {
	if ParseBillPeriod("week") != BillWeekly || ParseBillPeriod("year") != BillYearly {
		t.Error("recognized bill periods misparsed")
	}
	if ParseBillPeriod("") != BillMonthly || ParseBillPeriod("decade") != BillMonthly {
		t.Error("bill period should default to monthly")
	}
}
