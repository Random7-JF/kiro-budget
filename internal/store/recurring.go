package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// ListRecurring returns all recurring rules for userID ordered by day of month
// then id.
func (r *Repo) ListRecurring(ctx context.Context, userID int64) ([]budget.RecurringRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, amount_cents, category, description, day_of_month
		 FROM recurring WHERE user_id = ? ORDER BY day_of_month ASC, id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list recurring: %w", err)
	}
	defer rows.Close()

	var out []budget.RecurringRule
	for rows.Next() {
		var (
			rule    budget.RecurringRule
			typeStr string
		)
		if err := rows.Scan(&rule.ID, &typeStr, &rule.AmountCents, &rule.Category, &rule.Description, &rule.DayOfMonth); err != nil {
			return nil, fmt.Errorf("store: scan recurring: %w", err)
		}
		rule.Type = budget.TxType(typeStr)
		out = append(out, rule)
	}
	return out, rows.Err()
}

// CreateRecurring inserts a new recurring rule owned by userID.
func (r *Repo) CreateRecurring(ctx context.Context, userID int64, rule budget.RecurringRule) (budget.RecurringRule, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO recurring (user_id, type, amount_cents, category, description, day_of_month)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, string(rule.Type), rule.AmountCents, rule.Category, rule.Description, rule.DayOfMonth)
	if err != nil {
		return budget.RecurringRule{}, fmt.Errorf("store: create recurring: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return budget.RecurringRule{}, fmt.Errorf("store: recurring id: %w", err)
	}
	rule.ID = id
	return rule, nil
}

// DeleteRecurring removes a recurring rule owned by userID. Removing a
// non-existent rule is a no-op.
func (r *Repo) DeleteRecurring(ctx context.Context, userID, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM recurring WHERE id = ? AND user_id = ?`, id, userID); err != nil {
		return fmt.Errorf("store: delete recurring: %w", err)
	}
	return nil
}

// PostRecurringForMonth materializes each recurring rule owned by userID into a
// concrete transaction for month m, dated on the rule's day-of-month (clamped
// to the month's length). It is idempotent: a rule already posted for the month
// is skipped. Returns the number of transactions created.
func (r *Repo) PostRecurringForMonth(ctx context.Context, userID int64, m budget.Month) (int, error) {
	posted := 0
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, type, amount_cents, category, description, day_of_month FROM recurring WHERE user_id = ?`, userID)
		if err != nil {
			return err
		}
		var rules []budget.RecurringRule
		for rows.Next() {
			var (
				rule    budget.RecurringRule
				typeStr string
			)
			if err := rows.Scan(&rule.ID, &typeStr, &rule.AmountCents, &rule.Category, &rule.Description, &rule.DayOfMonth); err != nil {
				rows.Close()
				return err
			}
			rule.Type = budget.TxType(typeStr)
			rules = append(rules, rule)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		ym := m.String()
		days := m.Days()
		for _, rule := range rules {
			var one int
			err := tx.QueryRowContext(ctx,
				`SELECT 1 FROM recurring_postings WHERE recurring_id = ? AND ym = ?`, rule.ID, ym).Scan(&one)
			if err == nil {
				continue // already posted
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}

			day := rule.DayOfMonth
			if day > days {
				day = days
			}
			date := fmt.Sprintf("%s-%02d", ym, day)

			if _, err := tx.ExecContext(ctx,
				`INSERT INTO transactions (user_id, type, amount_cents, date, category, description)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				userID, string(rule.Type), rule.AmountCents, date, rule.Category, rule.Description); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO recurring_postings (recurring_id, ym) VALUES (?, ?)`, rule.ID, ym); err != nil {
				return err
			}
			posted++
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("store: post recurring: %w", err)
	}
	return posted, nil
}
