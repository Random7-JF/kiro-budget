package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// ListRecurring returns all recurring rules ordered by day of month then id.
func (r *Repo) ListRecurring(ctx context.Context) ([]budget.RecurringRule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, amount_cents, category, description, day_of_month
		 FROM recurring ORDER BY day_of_month ASC, id ASC`)
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

// CreateRecurring inserts a new recurring rule and returns it with its assigned id.
func (r *Repo) CreateRecurring(ctx context.Context, rule budget.RecurringRule) (budget.RecurringRule, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO recurring (type, amount_cents, category, description, day_of_month)
		 VALUES (?, ?, ?, ?, ?)`,
		string(rule.Type), rule.AmountCents, rule.Category, rule.Description, rule.DayOfMonth)
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

// DeleteRecurring removes a recurring rule. Removing a non-existent rule is a no-op.
func (r *Repo) DeleteRecurring(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM recurring WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete recurring: %w", err)
	}
	return nil
}

// PostRecurringForMonth materializes each active recurring rule into a concrete
// transaction for month m, dated on the rule's day-of-month (clamped to the
// month's length). It is idempotent: a rule already posted for the month is
// skipped (tracked in recurring_postings). The whole operation runs in a single
// transaction and returns the number of transactions created.
func (r *Repo) PostRecurringForMonth(ctx context.Context, m budget.Month) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: begin post recurring: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx,
		`SELECT id, type, amount_cents, category, description, day_of_month FROM recurring`)
	if err != nil {
		return 0, fmt.Errorf("store: load recurring: %w", err)
	}
	var rules []budget.RecurringRule
	for rows.Next() {
		var (
			rule    budget.RecurringRule
			typeStr string
		)
		if err := rows.Scan(&rule.ID, &typeStr, &rule.AmountCents, &rule.Category, &rule.Description, &rule.DayOfMonth); err != nil {
			rows.Close()
			return 0, fmt.Errorf("store: scan recurring: %w", err)
		}
		rule.Type = budget.TxType(typeStr)
		rules = append(rules, rule)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	ym := m.String()
	days := m.Days()
	posted := 0

	for _, rule := range rules {
		// Skip rules already posted for this month.
		var one int
		err := tx.QueryRowContext(ctx,
			`SELECT 1 FROM recurring_postings WHERE recurring_id = ? AND ym = ?`,
			rule.ID, ym).Scan(&one)
		if err == nil {
			continue // already posted
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("store: check posting: %w", err)
		}

		day := rule.DayOfMonth
		if day > days {
			day = days
		}
		date := fmt.Sprintf("%s-%02d", ym, day)

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO transactions (type, amount_cents, date, category, description)
			 VALUES (?, ?, ?, ?, ?)`,
			string(rule.Type), rule.AmountCents, date, rule.Category, rule.Description); err != nil {
			return 0, fmt.Errorf("store: post recurring txn: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO recurring_postings (recurring_id, ym) VALUES (?, ?)`,
			rule.ID, ym); err != nil {
			return 0, fmt.Errorf("store: record posting: %w", err)
		}
		posted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: commit post recurring: %w", err)
	}
	committed = true
	return posted, nil
}
