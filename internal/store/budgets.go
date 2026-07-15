package store

import (
	"context"
	"fmt"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// ListBudgets returns all configured per-category monthly budgets for userID,
// ordered by category.
func (r *Repo) ListBudgets(ctx context.Context, userID int64) ([]budget.CategoryBudget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT category, amount_cents FROM budgets WHERE user_id = ? ORDER BY category ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list budgets: %w", err)
	}
	defer rows.Close()

	var out []budget.CategoryBudget
	for rows.Next() {
		var b budget.CategoryBudget
		if err := rows.Scan(&b.Category, &b.LimitCents); err != nil {
			return nil, fmt.Errorf("store: scan budget: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetBudget creates or updates the monthly limit for a category owned by userID.
func (r *Repo) SetBudget(ctx context.Context, userID int64, category string, cents int64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO budgets (user_id, category, amount_cents) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, category) DO UPDATE SET amount_cents = excluded.amount_cents`,
		userID, category, cents)
	if err != nil {
		return fmt.Errorf("store: set budget: %w", err)
	}
	return nil
}

// DeleteBudget removes a category budget owned by userID. Removing a
// non-existent budget is a no-op.
func (r *Repo) DeleteBudget(ctx context.Context, userID int64, category string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM budgets WHERE user_id = ? AND category = ?`, userID, category); err != nil {
		return fmt.Errorf("store: delete budget: %w", err)
	}
	return nil
}
