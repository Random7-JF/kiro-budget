package budget

import (
	"reflect"
	"testing"
	"time"
)

func tx(id int64, typ TxType, cents int64, category string) Transaction {
	return Transaction{
		ID:          id,
		Type:        typ,
		AmountCents: cents,
		Date:        time.Date(2024, 5, 13, 0, 0, 0, 0, time.UTC),
		Category:    category,
	}
}

func TestSummaryEmptyInputYieldsZeroTotals(t *testing.T) {
	got := Summary(nil)
	if got.TotalIncomeCents != 0 || got.TotalExpenseCents != 0 || got.NetBalanceCents != 0 {
		t.Errorf("expected zero totals for empty input, got %+v", got)
	}
	if len(got.ByCategory) != 0 {
		t.Errorf("expected no category entries for empty input, got %+v", got.ByCategory)
	}
}

func TestSummaryTotalsAndNetBalance(t *testing.T) {
	txns := []Transaction{
		tx(1, TypeIncome, 500000, "Salary"),
		tx(2, TypeIncome, 10000, "Gifts"),
		tx(3, TypeExpense, 12500, "Groceries"),
		tx(4, TypeExpense, 7500, "Rent"),
	}
	got := Summary(txns)
	if got.TotalIncomeCents != 510000 {
		t.Errorf("total income = %d, want 510000", got.TotalIncomeCents)
	}
	if got.TotalExpenseCents != 20000 {
		t.Errorf("total expense = %d, want 20000", got.TotalExpenseCents)
	}
	if got.NetBalanceCents != 490000 {
		t.Errorf("net balance = %d, want 490000", got.NetBalanceCents)
	}
}

func TestSummaryNegativeNetBalanceAllowed(t *testing.T) {
	txns := []Transaction{
		tx(1, TypeIncome, 1000, "Salary"),
		tx(2, TypeExpense, 5000, "Rent"),
	}
	got := Summary(txns)
	if got.NetBalanceCents != -4000 {
		t.Errorf("net balance = %d, want -4000 (negative allowed)", got.NetBalanceCents)
	}
}

func TestSummaryCategoryBreakdownAggregatesExpensesOnly(t *testing.T) {
	txns := []Transaction{
		tx(1, TypeExpense, 1000, "Groceries"),
		tx(2, TypeExpense, 2000, "Groceries"),
		tx(3, TypeExpense, 500, "Rent"),
		tx(4, TypeIncome, 9999, "Groceries"), // income must not appear in breakdown
	}
	got := Summary(txns)
	want := []CategoryExpense{
		{Category: "Groceries", TotalCents: 3000},
		{Category: "Rent", TotalCents: 500},
	}
	if !reflect.DeepEqual(got.ByCategory, want) {
		t.Errorf("ByCategory = %+v, want %+v", got.ByCategory, want)
	}
}

func TestSummaryCategoryBreakdownDeterministicOrder(t *testing.T) {
	txns := []Transaction{
		tx(1, TypeExpense, 100, "Zebra"),
		tx(2, TypeExpense, 100, "Apple"),
		tx(3, TypeExpense, 100, "Mango"),
	}
	got := Summary(txns)
	wantOrder := []string{"Apple", "Mango", "Zebra"}
	if len(got.ByCategory) != len(wantOrder) {
		t.Fatalf("got %d categories, want %d", len(got.ByCategory), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got.ByCategory[i].Category != name {
			t.Errorf("ByCategory[%d] = %q, want %q", i, got.ByCategory[i].Category, name)
		}
	}
}

func TestSummaryCategoryTotalsSumToOverallExpense(t *testing.T) {
	txns := []Transaction{
		tx(1, TypeExpense, 1234, "A"),
		tx(2, TypeExpense, 5678, "B"),
		tx(3, TypeExpense, 9012, "A"),
		tx(4, TypeIncome, 4321, "C"),
	}
	got := Summary(txns)
	var sum int64
	for _, c := range got.ByCategory {
		sum += c.TotalCents
	}
	if sum != got.TotalExpenseCents {
		t.Errorf("sum of category totals = %d, want overall expense %d", sum, got.TotalExpenseCents)
	}
}
