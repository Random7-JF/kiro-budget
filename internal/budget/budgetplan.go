package budget

import "sort"

// CategoryBudget is a configured monthly spending limit for a category.
type CategoryBudget struct {
	Category   string
	LimitCents int64
}

// BudgetProgress pairs a category's configured monthly limit with the amount
// spent against it in a given month.
type BudgetProgress struct {
	Category   string
	LimitCents int64
	SpentCents int64
}

// RemainingCents is the amount left under the limit; negative when over budget.
func (b BudgetProgress) RemainingCents() int64 { return b.LimitCents - b.SpentCents }

// Over reports whether spending has exceeded the limit.
func (b BudgetProgress) Over() bool { return b.SpentCents > b.LimitCents }

// Percent is spent as a percentage of the limit (may exceed 100).
func (b BudgetProgress) Percent() int {
	if b.LimitCents <= 0 {
		return 0
	}
	p := int(b.SpentCents * 100 / b.LimitCents)
	if p < 0 {
		p = 0
	}
	return p
}

// BarPercent is Percent clamped to the 0..100 range, for the progress bar width.
func (b BudgetProgress) BarPercent() int {
	p := b.Percent()
	if p > 100 {
		return 100
	}
	return p
}

// Near reports whether spending is at or above 80% of the limit (but not over),
// used to warn before going over budget.
func (b BudgetProgress) Near() bool {
	return !b.Over() && b.Percent() >= 80
}

// BudgetStatus builds the per-category progress for the configured budgets,
// using spentByCategory (typically the month's expense breakdown). The result
// is sorted by category name.
func BudgetStatus(spentByCategory map[string]int64, budgets []CategoryBudget) []BudgetProgress {
	out := make([]BudgetProgress, 0, len(budgets))
	for _, b := range budgets {
		out = append(out, BudgetProgress{
			Category:   b.Category,
			LimitCents: b.LimitCents,
			SpentCents: spentByCategory[b.Category],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

// SpentByCategory reduces a summary's expense breakdown to a category->cents map.
func SpentByCategory(s DashboardSummary) map[string]int64 {
	m := make(map[string]int64, len(s.ByCategory))
	for _, c := range s.ByCategory {
		m[c.Category] = c.TotalCents
	}
	return m
}
