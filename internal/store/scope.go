package store

import (
	"context"

	"github.com/budget-tracker/budget-tracker/internal/budget"
)

// UserScope is a view of the repository bound to a single user. Every operation
// it exposes is automatically scoped to that user's data, so HTTP handlers can
// work without threading a user id through every call. It implements the data
// interfaces consumed by the http package.
type UserScope struct {
	repo   *Repo
	userID int64
}

// ForUser returns a UserScope bound to userID.
func (r *Repo) ForUser(userID int64) *UserScope {
	return &UserScope{repo: r, userID: userID}
}

// UserID returns the user this scope is bound to.
func (s *UserScope) UserID() int64 { return s.userID }

// --- Transactions ---

func (s *UserScope) ListTransactions(ctx context.Context) ([]budget.Transaction, error) {
	return s.repo.ListTransactions(ctx, s.userID)
}

func (s *UserScope) CreateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return s.repo.CreateTransaction(ctx, s.userID, tx)
}

func (s *UserScope) UpdateTransaction(ctx context.Context, tx budget.Transaction) (budget.Transaction, error) {
	return s.repo.UpdateTransaction(ctx, s.userID, tx)
}

func (s *UserScope) DeleteTransaction(ctx context.Context, id int64) error {
	return s.repo.DeleteTransaction(ctx, s.userID, id)
}

func (s *UserScope) GetTransaction(ctx context.Context, id int64) (budget.Transaction, error) {
	return s.repo.GetTransaction(ctx, s.userID, id)
}

// --- Budgets ---

func (s *UserScope) ListBudgets(ctx context.Context) ([]budget.CategoryBudget, error) {
	return s.repo.ListBudgets(ctx, s.userID)
}

func (s *UserScope) SetBudget(ctx context.Context, category string, cents int64) error {
	return s.repo.SetBudget(ctx, s.userID, category, cents)
}

func (s *UserScope) DeleteBudget(ctx context.Context, category string) error {
	return s.repo.DeleteBudget(ctx, s.userID, category)
}

// --- Recurring ---

func (s *UserScope) ListRecurring(ctx context.Context) ([]budget.RecurringRule, error) {
	return s.repo.ListRecurring(ctx, s.userID)
}

func (s *UserScope) CreateRecurring(ctx context.Context, rule budget.RecurringRule) (budget.RecurringRule, error) {
	return s.repo.CreateRecurring(ctx, s.userID, rule)
}

func (s *UserScope) DeleteRecurring(ctx context.Context, id int64) error {
	return s.repo.DeleteRecurring(ctx, s.userID, id)
}

func (s *UserScope) PostRecurringForMonth(ctx context.Context, m budget.Month) (int, error) {
	return s.repo.PostRecurringForMonth(ctx, s.userID, m)
}

// --- Bills ---

func (s *UserScope) ListBills(ctx context.Context) ([]budget.Bill, error) {
	return s.repo.ListBills(ctx, s.userID)
}

func (s *UserScope) CreateBill(ctx context.Context, b budget.Bill) (budget.Bill, error) {
	return s.repo.CreateBill(ctx, s.userID, b)
}

func (s *UserScope) DeleteBill(ctx context.Context, id int64) error {
	return s.repo.DeleteBill(ctx, s.userID, id)
}
