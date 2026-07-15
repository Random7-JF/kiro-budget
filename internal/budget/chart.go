package budget

import "sort"

// ChartBar is one bar in the spending-by-category chart: the category, the
// total expense in cents, and its width as a percentage of the largest
// category (0..100) so the view can render a proportional bar.
type ChartBar struct {
	Category string
	Cents    int64
	Percent  int
}

// CategoryChart builds the spending-by-category chart data from a summary's
// expense breakdown, ordered from largest to smallest spend. Bar widths are
// scaled relative to the largest category.
func CategoryChart(s DashboardSummary) []ChartBar {
	var max int64
	for _, c := range s.ByCategory {
		if c.TotalCents > max {
			max = c.TotalCents
		}
	}

	bars := make([]ChartBar, 0, len(s.ByCategory))
	for _, c := range s.ByCategory {
		pct := 0
		if max > 0 {
			pct = int(c.TotalCents * 100 / max)
		}
		bars = append(bars, ChartBar{Category: c.Category, Cents: c.TotalCents, Percent: pct})
	}

	sort.Slice(bars, func(i, j int) bool {
		if bars[i].Cents != bars[j].Cents {
			return bars[i].Cents > bars[j].Cents
		}
		return bars[i].Category < bars[j].Category
	})
	return bars
}
