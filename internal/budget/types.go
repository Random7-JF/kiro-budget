package budget

import "time"

// TxType is the kind of a transaction: expense or income.
type TxType string

const (
	// TypeExpense marks a transaction that reduces available funds.
	TypeExpense TxType = "expense"
	// TypeIncome marks a transaction that increases available funds.
	TypeIncome TxType = "income"
)

// TimePeriod is a grouping interval for viewing transactions.
type TimePeriod string

const (
	// PeriodDay groups transactions by individual calendar date.
	PeriodDay TimePeriod = "day"
	// PeriodWeek groups transactions by calendar week starting Monday.
	PeriodWeek TimePeriod = "week"
	// PeriodMonth groups transactions by calendar month.
	PeriodMonth TimePeriod = "month"
)

// SortField identifies which transaction field a sort applies to.
type SortField string

const (
	// SortByDate sorts by Transaction_Date.
	SortByDate SortField = "date"
	// SortByAmount sorts by Amount.
	SortByAmount SortField = "amount"
)

// SortDir is the direction of a sort.
type SortDir string

const (
	// SortAsc orders from lowest/earliest to highest/latest.
	SortAsc SortDir = "asc"
	// SortDesc orders from highest/latest to lowest/earliest.
	SortDesc SortDir = "desc"
)

// TypeFilter selects which transaction types to display. The empty value
// (FilterAll) means no filter is applied and both types are shown.
type TypeFilter string

const (
	// FilterAll applies no type filter; both expenses and income are shown.
	FilterAll TypeFilter = ""
	// FilterExpense shows only expense transactions.
	FilterExpense TypeFilter = "expense"
	// FilterIncome shows only income transactions.
	FilterIncome TypeFilter = "income"
)

// Transaction is a single, normalized financial record.
type Transaction struct {
	ID          int64
	Type        TxType
	AmountCents int64     // non-negative magnitude; type distinguishes income/expense
	Date        time.Time // date-only (UTC midnight); rendered as YYYY-MM-DD
	Category    string
	Description string // may be empty
}

// TransactionInput is raw, unvalidated user input for creating or editing a
// transaction.
type TransactionInput struct {
	ID          int64 // 0 for create
	Type        TxType
	AmountText  string // raw user input, parsed/validated
	DateText    string // raw ISO 8601 text
	Category    string
	Description string
}

// FieldError names a single invalid input field and its validation message.
type FieldError struct {
	Field   string // "amount" | "date" | "category"
	Message string
}

// CategoryExpense is the total expense for a single category.
type CategoryExpense struct {
	Category   string
	TotalCents int64
}

// DashboardSummary holds aggregated financial figures for a set of
// transactions within a time period.
type DashboardSummary struct {
	TotalIncomeCents  int64
	TotalExpenseCents int64
	NetBalanceCents   int64 // TotalIncomeCents - TotalExpenseCents; may be negative
	ByCategory        []CategoryExpense
}

// Group is a set of transactions bucketed into a time period, along with that
// bucket's summary.
type Group struct {
	Key     string // e.g. "2024-05-13", "2024-W20", "2024-05"
	Label   string
	Summary DashboardSummary
	Items   []Transaction
}
