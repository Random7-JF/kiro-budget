package budget

import "sort"

// BillFrequency is how often a bill recurs.
type BillFrequency string

const (
	FreqWeekly    BillFrequency = "weekly"
	FreqBiweekly  BillFrequency = "biweekly"
	FreqMonthly   BillFrequency = "monthly"
	FreqQuarterly BillFrequency = "quarterly"
	FreqYearly    BillFrequency = "yearly"
)

// PerYear returns how many times the bill occurs per year, or 0 for an
// unrecognized frequency.
func (f BillFrequency) PerYear() int {
	switch f {
	case FreqWeekly:
		return 52
	case FreqBiweekly:
		return 26
	case FreqMonthly:
		return 12
	case FreqQuarterly:
		return 4
	case FreqYearly:
		return 1
	default:
		return 0
	}
}

// Label renders the frequency for display.
func (f BillFrequency) Label() string {
	switch f {
	case FreqWeekly:
		return "Weekly"
	case FreqBiweekly:
		return "Every 2 weeks"
	case FreqMonthly:
		return "Monthly"
	case FreqQuarterly:
		return "Quarterly"
	case FreqYearly:
		return "Yearly"
	default:
		return string(f)
	}
}

// ParseFrequency parses a frequency string, reporting whether it was recognized.
func ParseFrequency(s string) (BillFrequency, bool) {
	f := BillFrequency(s)
	if f.PerYear() == 0 {
		return "", false
	}
	return f, true
}

// Bill is a recurring obligation to a payee.
type Bill struct {
	ID          int64
	Payee       string
	AmountCents int64
	Frequency   BillFrequency
	Category    string
	DueDay      int // 0 when unset; 1..31 otherwise (informational)
}

// BillPeriod is the interval a bills breakdown is normalized to.
type BillPeriod string

const (
	BillWeekly  BillPeriod = "week"
	BillMonthly BillPeriod = "month"
	BillYearly  BillPeriod = "year"
)

// ParseBillPeriod parses a bill-period string, defaulting to monthly.
func ParseBillPeriod(s string) BillPeriod {
	switch BillPeriod(s) {
	case BillWeekly, BillMonthly, BillYearly:
		return BillPeriod(s)
	default:
		return BillMonthly
	}
}

// Label renders the bill period for display ("Weekly"/"Monthly"/"Yearly").
func (p BillPeriod) Label() string {
	switch p {
	case BillWeekly:
		return "Weekly"
	case BillYearly:
		return "Yearly"
	default:
		return "Monthly"
	}
}

// divRound divides n by d, rounding to the nearest integer (n, d >= 0).
func divRound(n, d int64) int64 {
	if d == 0 {
		return 0
	}
	return (n + d/2) / d
}

// NormalizedCents converts a bill's per-occurrence amount into the equivalent
// cost over the given viewing period (e.g. a $1200/year bill is $100/month).
func NormalizedCents(amountCents int64, freq BillFrequency, period BillPeriod) int64 {
	perYear := int64(freq.PerYear())
	if perYear == 0 {
		return 0
	}
	yearly := amountCents * perYear
	switch period {
	case BillYearly:
		return yearly
	case BillWeekly:
		return divRound(yearly, 52)
	default: // monthly
		return divRound(yearly, 12)
	}
}

// BillBreakdownItem is one bill's normalized cost for a period.
type BillBreakdownItem struct {
	Bill        Bill
	PeriodCents int64
}

// BillBreakdown is the total and per-bill normalized costs for a period.
type BillBreakdown struct {
	Period     BillPeriod
	TotalCents int64
	Items      []BillBreakdownItem
}

// BuildBillBreakdown normalizes every bill to the given period, totals them,
// and returns the items sorted from most to least expensive for that period.
func BuildBillBreakdown(bills []Bill, period BillPeriod) BillBreakdown {
	items := make([]BillBreakdownItem, 0, len(bills))
	var total int64
	for _, b := range bills {
		c := NormalizedCents(b.AmountCents, b.Frequency, period)
		total += c
		items = append(items, BillBreakdownItem{Bill: b, PeriodCents: c})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PeriodCents != items[j].PeriodCents {
			return items[i].PeriodCents > items[j].PeriodCents
		}
		return items[i].Bill.Payee < items[j].Bill.Payee
	})
	return BillBreakdown{Period: period, TotalCents: total, Items: items}
}
