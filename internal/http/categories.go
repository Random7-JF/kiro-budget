package http

import (
	"sort"
	"strings"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// defaultCategories are sensible starter suggestions offered in the category
// autocomplete even before the user has entered any transactions. They cover
// common personal-finance expense and income labels.
var defaultCategories = []string{
	"Dining", "Entertainment", "Gifts", "Groceries", "Health",
	"Housing", "Insurance", "Rent", "Salary", "Savings",
	"Shopping", "Subscriptions", "Transportation", "Travel", "Utilities",
}

// categoryOptions builds the suggestion list for the category input: the
// default categories merged with every distinct category already used in the
// stored transactions. Matching is case-insensitive (the first-seen spelling
// wins) and the result is sorted alphabetically. The input remains free-text,
// so these are suggestions only.
func categoryOptions(txns []budget.Transaction) []string {
	seen := make(map[string]bool)
	var out []string

	add := func(c string) {
		c = strings.TrimSpace(c)
		if c == "" {
			return
		}
		key := strings.ToLower(c)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
	}

	for _, c := range defaultCategories {
		add(c)
	}
	for _, t := range txns {
		add(t.Category)
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}
