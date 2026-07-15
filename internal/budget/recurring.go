package budget

// RecurringRule is a template for a transaction that repeats monthly on a given
// day of the month. Posting a rule for a month materializes a concrete
// Transaction dated on that day (clamped to the month's length).
type RecurringRule struct {
	ID          int64
	Type        TxType
	AmountCents int64
	Category    string
	Description string
	DayOfMonth  int // 1..31; clamped to the month's last day when posted
}
