package store

import (
	"context"
	"fmt"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// ListBills returns all bills ordered by payee.
func (r *Repo) ListBills(ctx context.Context) ([]budget.Bill, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, payee, amount_cents, frequency, category, due_day
		 FROM bills ORDER BY payee ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list bills: %w", err)
	}
	defer rows.Close()

	var out []budget.Bill
	for rows.Next() {
		var (
			b       budget.Bill
			freqStr string
		)
		if err := rows.Scan(&b.ID, &b.Payee, &b.AmountCents, &freqStr, &b.Category, &b.DueDay); err != nil {
			return nil, fmt.Errorf("store: scan bill: %w", err)
		}
		b.Frequency = budget.BillFrequency(freqStr)
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateBill inserts a new bill and returns it with its assigned id.
func (r *Repo) CreateBill(ctx context.Context, b budget.Bill) (budget.Bill, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO bills (payee, amount_cents, frequency, category, due_day)
		 VALUES (?, ?, ?, ?, ?)`,
		b.Payee, b.AmountCents, string(b.Frequency), b.Category, b.DueDay)
	if err != nil {
		return budget.Bill{}, fmt.Errorf("store: create bill: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return budget.Bill{}, fmt.Errorf("store: bill id: %w", err)
	}
	b.ID = id
	return b, nil
}

// DeleteBill removes a bill by id. Removing a non-existent bill is a no-op.
func (r *Repo) DeleteBill(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM bills WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete bill: %w", err)
	}
	return nil
}
