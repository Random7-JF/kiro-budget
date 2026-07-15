package budget

import (
	"fmt"
	"sort"
	"time"
)

// GroupByPeriod buckets transactions into time-period groups by day, week (starting
// Monday), or month. Each returned Group carries the transactions that fall in
// its bucket, its own Summary (totals, net balance, and category breakdown for
// just that bucket), a stable Key, and a human-readable Label.
//
// Groups are ordered from the most recent bucket to the least recent bucket.
// Within a group, transactions preserve their order of appearance in the input.
//
// Bucketing keys:
//   - day:   "2006-01-02"
//   - week:  ISO year-week "2006-W01" (Monday start, via time.ISOWeek)
//   - month: "2006-01"
//
// An unrecognized period defaults to month, mirroring ParsePeriod's behavior.
// An empty (or nil) input yields an empty (non-nil) slice.
//
// Requirements: 7.1, 7.2, 7.3, 7.4
func GroupByPeriod(txns []Transaction, period TimePeriod) []Group {
	// index maps a bucket key to its position in the groups slice so we can
	// accumulate items while preserving input order within each bucket.
	index := make(map[string]int)
	groups := make([]Group, 0)

	for _, t := range txns {
		key, label := bucketKeyLabel(t.Date, period)

		pos, ok := index[key]
		if !ok {
			pos = len(groups)
			index[key] = pos
			groups = append(groups, Group{
				Key:   key,
				Label: label,
				Items: make([]Transaction, 0, 1),
			})
		}
		groups[pos].Items = append(groups[pos].Items, t)
	}

	// Order groups from most-recent bucket to least-recent. The chosen key
	// formats are all lexicographically ordered in calendar order (ISO
	// year-week ordering is chronological), so a descending key sort yields
	// most-recent-first.
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Key > groups[j].Key
	})

	// Compute each group's own summary totals and net balance.
	for i := range groups {
		groups[i].Summary = Summary(groups[i].Items)
	}

	return groups
}

// bucketKeyLabel returns the bucket key and human-readable label for a date
// under the given period. An unrecognized period is treated as month.
func bucketKeyLabel(date time.Time, period TimePeriod) (key, label string) {
	switch period {
	case PeriodDay:
		return date.Format("2006-01-02"), date.Format("Jan 2, 2006")
	case PeriodWeek:
		isoYear, isoWeek := date.ISOWeek()
		key = fmt.Sprintf("%04d-W%02d", isoYear, isoWeek)
		// Label anchors the week to its Monday start.
		offset := (int(date.Weekday()) + 6) % 7 // days since Monday
		monday := date.AddDate(0, 0, -offset)
		label = fmt.Sprintf("Week of %s", monday.Format("Jan 2, 2006"))
		return key, label
	case PeriodMonth:
		return date.Format("2006-01"), date.Format("January 2006")
	default:
		// Unrecognized period defaults to month.
		return date.Format("2006-01"), date.Format("January 2006")
	}
}
