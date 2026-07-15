package budget

import (
	"fmt"
	"time"
)

// Month identifies a calendar month (year + month), used to scope the
// dashboard, budgets, chart, and log to a single month.
type Month struct {
	Year int
	M    time.Month
}

// MonthOf returns the Month containing t.
func MonthOf(t time.Time) Month { return Month{Year: t.Year(), M: t.Month()} }

// String renders the month as "YYYY-MM" (used in query params and keys).
func (m Month) String() string { return fmt.Sprintf("%04d-%02d", m.Year, int(m.M)) }

// Label renders the month for display, e.g. "January 2006".
func (m Month) Label() string {
	return time.Date(m.Year, m.M, 1, 0, 0, 0, 0, time.UTC).Format("January 2006")
}

// Prev returns the month immediately before m.
func (m Month) Prev() Month {
	t := time.Date(m.Year, m.M, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	return Month{Year: t.Year(), M: t.Month()}
}

// Next returns the month immediately after m.
func (m Month) Next() Month {
	t := time.Date(m.Year, m.M, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	return Month{Year: t.Year(), M: t.Month()}
}

// Days returns the number of days in the month.
func (m Month) Days() int {
	return time.Date(m.Year, m.M+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// Contains reports whether t falls within this month.
func (m Month) Contains(t time.Time) bool {
	return t.Year() == m.Year && t.Month() == m.M
}

// ParseMonth parses a "YYYY-MM" string into a Month.
func ParseMonth(s string) (Month, bool) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return Month{}, false
	}
	return Month{Year: t.Year(), M: t.Month()}, true
}

// FilterMonth returns only the transactions whose date falls within m.
func FilterMonth(txns []Transaction, m Month) []Transaction {
	out := make([]Transaction, 0, len(txns))
	for _, t := range txns {
		if m.Contains(t.Date) {
			out = append(out, t)
		}
	}
	return out
}

// LatestMonth returns the month of the most recent transaction, or fallback
// when there are no transactions.
func LatestMonth(txns []Transaction, fallback Month) Month {
	found := false
	var latest time.Time
	for _, t := range txns {
		if !found || t.Date.After(latest) {
			latest = t.Date
			found = true
		}
	}
	if !found {
		return fallback
	}
	return MonthOf(latest)
}
