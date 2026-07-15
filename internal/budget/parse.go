package budget

// ParsePeriod parses a raw query-parameter value into a TimePeriod. It returns
// the corresponding period only when the value is exactly "day", "week", or
// "month"; for an absent (empty) or otherwise unrecognized value it defaults to
// PeriodMonth.
func ParsePeriod(s string) TimePeriod {
	switch TimePeriod(s) {
	case PeriodDay, PeriodWeek, PeriodMonth:
		return TimePeriod(s)
	default:
		return PeriodMonth
	}
}

// ParseSort parses raw sort field and direction query-parameter values into a
// SortField and SortDir. It returns the selected pair only when BOTH the field
// is a recognized value ("date" or "amount") AND the direction is a recognized
// value ("asc" or "desc"). If either value is absent or unrecognized, it falls
// back to the default ordering of Transaction_Date descending
// (SortByDate, SortDesc).
func ParseSort(field, dir string) (SortField, SortDir) {
	f, okField := parseSortField(field)
	d, okDir := parseSortDir(dir)
	if okField && okDir {
		return f, d
	}
	return SortByDate, SortDesc
}

func parseSortField(field string) (SortField, bool) {
	switch SortField(field) {
	case SortByDate, SortByAmount:
		return SortField(field), true
	default:
		return SortByDate, false
	}
}

func parseSortDir(dir string) (SortDir, bool) {
	switch SortDir(dir) {
	case SortAsc, SortDesc:
		return SortDir(dir), true
	default:
		return SortDesc, false
	}
}
