package budget

import (
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// dateProbe holds a generated Transaction_Date candidate string. The candidate
// may be a valid ISO 8601 calendar date or an intentionally malformed one; the
// test decides the expected outcome using time.Parse as an independent oracle.
type dateProbe struct {
	text string
}

// genValidISODate generates syntactically valid ISO 8601 calendar dates in
// YYYY-MM-DD format spanning a wide range of years. Dates are constructed by
// adding a day offset to a base date so every produced string is a real
// calendar date (never an impossible one like 2024-02-30).
func genValidISODate() *rapid.Generator[dateProbe] {
	base := time.Date(1000, time.January, 1, 0, 0, 0, 0, time.UTC)
	return rapid.Custom(func(t *rapid.T) dateProbe {
		// Roughly 0..~8000 years past the base, keeping 4-digit years.
		offset := rapid.IntRange(0, 2_900_000).Draw(t, "dayOffset")
		d := base.AddDate(0, 0, offset)
		return dateProbe{text: d.Format(dateLayout)}
	})
}

// genInvalidDate generates strings that are NOT valid ISO 8601 calendar dates:
// empty/blank, wrong separators or ordering, impossible calendar dates, and
// arbitrary non-date text.
func genInvalidDate() *rapid.Generator[dateProbe] {
	return rapid.Custom(func(t *rapid.T) dateProbe {
		kind := rapid.IntRange(0, 5).Draw(t, "kind")
		switch kind {
		case 0: // empty / blank
			return dateProbe{text: rapid.SampledFrom([]string{"", " ", "  ", "\t"}).Draw(t, "blank")}
		case 1: // wrong separators / ordering
			return dateProbe{text: rapid.SampledFrom([]string{
				"2024/01/01", "01-01-2024", "2024.01.01", "20240101",
				"2024-1-1", "24-01-01", "2024-01-01T00:00:00", "2024_01_01",
			}).Draw(t, "wrongFormat")}
		case 2: // impossible month
			month := rapid.IntRange(13, 20).Draw(t, "badMonth")
			day := rapid.IntRange(1, 28).Draw(t, "day")
			return dateProbe{text: fmt.Sprintf("2024-%02d-%02d", month, day)}
		case 3: // zero or impossible day
			return dateProbe{text: rapid.SampledFrom([]string{
				"2024-00-10", "2024-01-00", "2024-02-30", "2024-04-31",
				"2023-02-29", "2024-06-31", "2024-11-31",
			}).Draw(t, "badDay")}
		case 4: // non-date text
			return dateProbe{text: rapid.StringMatching(`[a-zA-Z ]{1,15}`).Draw(t, "text")}
		default: // numeric-ish garbage
			return dateProbe{text: rapid.SampledFrom([]string{
				"2024-13-01", "0000-00-00", "99999-01-01", "2024-01-32",
				"-2024-01-01", "2024-01-", "-01-01", "2024--01",
			}).Draw(t, "garbage")}
		}
	})
}

// genDateCandidate mixes valid and invalid date strings so the property is
// exercised on both accepted and rejected inputs.
func genDateCandidate() *rapid.Generator[dateProbe] {
	return rapid.OneOf(genValidISODate(), genInvalidDate())
}

// dateErrorPresent reports whether the given field errors contain an error
// whose Field is "date".
func dateErrorPresent(errs []FieldError) bool {
	for _, e := range errs {
		if e.Field == "date" {
			return true
		}
	}
	return false
}

// Feature: budget-tracker, Property 3: Date validation accepts only valid ISO 8601 calendar dates
func TestValidateDateProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		probe := genDateCandidate().Draw(t, "date")

		// Isolate the date field: always supply a valid amount and category so
		// only the date determines whether a "date" field error appears.
		input := TransactionInput{
			Type:       TypeExpense,
			AmountText: "10.00",
			DateText:   probe.text,
			Category:   "Food",
		}

		_, errs := Validate(input)

		// Oracle: a Transaction_Date is valid iff time.Parse accepts it under
		// the strict YYYY-MM-DD layout.
		_, oracleErr := time.Parse(dateLayout, probe.text)
		wantValid := oracleErr == nil

		gotDateErr := dateErrorPresent(errs)

		if wantValid && gotDateErr {
			t.Fatalf("Validate rejected valid date %q with a date FieldError; want accepted", probe.text)
		}
		if !wantValid && !gotDateErr {
			t.Fatalf("Validate accepted invalid date %q; want a FieldError with Field==\"date\"", probe.text)
		}
	})
}
