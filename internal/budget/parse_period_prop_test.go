package budget

import (
	"testing"

	"pgregory.net/rapid"
)

// Feature: budget-tracker, Property 17: Time_Period parsing defaults to month
func TestParsePeriodProperty(t *testing.T) {
	// Exact matches must map to their corresponding TimePeriod.
	exact := map[string]TimePeriod{
		"day":   PeriodDay,
		"week":  PeriodWeek,
		"month": PeriodMonth,
	}
	for s, want := range exact {
		if got := ParsePeriod(s); got != want {
			t.Fatalf("ParsePeriod(%q) = %q, want %q", s, got, want)
		}
	}

	rapid.Check(t, func(rt *rapid.T) {
		s := rapid.String().Draw(rt, "period")
		got := ParsePeriod(s)
		if want, ok := exact[s]; ok {
			// Exact recognized value: must return the selected period.
			if got != want {
				rt.Fatalf("ParsePeriod(%q) = %q, want %q", s, got, want)
			}
			return
		}
		// Any other (absent/unrecognized) value: must default to month.
		if got != PeriodMonth {
			rt.Fatalf("ParsePeriod(%q) = %q, want %q (default)", s, got, PeriodMonth)
		}
	})
}
