package budget

import "sort"

// Summary computes aggregated financial figures for a set of transactions:
// total income, total expense, net balance (income - expense, which may be
// negative), and a per-category expense breakdown.
//
// The per-category breakdown contains exactly one entry for each category that
// has at least one expense, and no entry for categories with no expense. Income
// transactions do not contribute to the category breakdown. The breakdown is
// ordered deterministically by category name (ascending).
//
// An empty (or nil) input yields zero totals and an empty breakdown.
//
// Requirements: 5.1, 5.2, 5.3, 5.4
func Summary(txns []Transaction) DashboardSummary {
	var totalIncome, totalExpense int64
	byCategory := make(map[string]int64)

	for _, t := range txns {
		switch t.Type {
		case TypeIncome:
			totalIncome += t.AmountCents
		case TypeExpense:
			totalExpense += t.AmountCents
			byCategory[t.Category] += t.AmountCents
		}
	}

	categories := make([]CategoryExpense, 0, len(byCategory))
	for category, total := range byCategory {
		categories = append(categories, CategoryExpense{
			Category:   category,
			TotalCents: total,
		})
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Category < categories[j].Category
	})

	return DashboardSummary{
		TotalIncomeCents:  totalIncome,
		TotalExpenseCents: totalExpense,
		NetBalanceCents:   totalIncome - totalExpense,
		ByCategory:        categories,
	}
}
