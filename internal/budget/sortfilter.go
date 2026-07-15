package budget

import "sort"

// Sort returns a sorted copy of txns ordered by the given field and direction.
// The input slice is not mutated.
//
// Supported fields are SortByDate and SortByAmount; supported directions are
// SortAsc and SortDesc. When two transactions have equal values for the
// selected sort field, they are ordered by their ID in ascending order.
//
// If the field or direction is not recognized, the default ordering of
// Transaction_Date descending is applied.
//
// Requirements: 6.3, 8.1, 8.2, 8.3, 8.4, 8.6, 8.7
func Sort(txns []Transaction, field SortField, dir SortDir) []Transaction {
	// Normalize unrecognized field/direction to the default: date descending.
	if (field != SortByDate && field != SortByAmount) ||
		(dir != SortAsc && dir != SortDesc) {
		field = SortByDate
		dir = SortDesc
	}

	sorted := make([]Transaction, len(txns))
	copy(sorted, txns)

	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]

		// less reports whether a should come before b, before applying
		// direction and tie-breaking.
		var cmp int // -1 if a<b, 0 if equal, +1 if a>b, on the sort field
		switch field {
		case SortByAmount:
			switch {
			case a.AmountCents < b.AmountCents:
				cmp = -1
			case a.AmountCents > b.AmountCents:
				cmp = 1
			}
		default: // SortByDate
			switch {
			case a.Date.Before(b.Date):
				cmp = -1
			case a.Date.After(b.Date):
				cmp = 1
			}
		}

		if cmp == 0 {
			// Tie-break by ascending ID regardless of sort direction.
			return a.ID < b.ID
		}
		if dir == SortAsc {
			return cmp < 0
		}
		return cmp > 0
	})

	return sorted
}

// Filter returns a copy of txns containing only transactions whose type matches
// typeFilter. When typeFilter is FilterAll (no filter), all transactions are
// returned. The input slice is not mutated and the relative order of retained
// transactions is preserved.
//
// Requirements: 6.5, 6.6
func Filter(txns []Transaction, typeFilter TypeFilter) []Transaction {
	result := make([]Transaction, 0, len(txns))

	for _, t := range txns {
		switch typeFilter {
		case FilterExpense:
			if t.Type == TypeExpense {
				result = append(result, t)
			}
		case FilterIncome:
			if t.Type == TypeIncome {
				result = append(result, t)
			}
		default: // FilterAll or any unrecognized value: no filter
			result = append(result, t)
		}
	}

	return result
}
