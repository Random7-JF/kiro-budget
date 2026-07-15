package budget

import (
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: budget-tracker, Property 10: Amount formatting always has exactly two decimals
//
// For any non-negative integer cents value, FormatAmount produces a string with
// exactly two digits after a single decimal point, and parsing that string back
// yields the same value (whole = cents/100, frac = cents%100).
func TestFormatAmountProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cents := rapid.Int64Range(0, MaxAmountCents).Draw(t, "cents")

		out := FormatAmount(cents)

		// There must be exactly one decimal point.
		dot := strings.IndexByte(out, '.')
		if dot < 0 {
			t.Fatalf("FormatAmount(%d) = %q has no decimal point", cents, out)
		}
		if strings.IndexByte(out[dot+1:], '.') >= 0 {
			t.Fatalf("FormatAmount(%d) = %q has more than one decimal point", cents, out)
		}

		intPart := out[:dot]
		fracPart := out[dot+1:]

		// Exactly two digits after the decimal point.
		if len(fracPart) != 2 {
			t.Fatalf("FormatAmount(%d) = %q has %d fractional digits, want 2", cents, out, len(fracPart))
		}

		// Both parts must be pure decimal digits (non-negative input has no sign).
		if intPart == "" || !allDigits(intPart) || !allDigits(fracPart) {
			t.Fatalf("FormatAmount(%d) = %q has non-digit content in integer/fraction parts", cents, out)
		}

		// Parsing back must reproduce the original value.
		whole, err := strconv.ParseInt(intPart, 10, 64)
		if err != nil {
			t.Fatalf("FormatAmount(%d) = %q: cannot parse integer part: %v", cents, out, err)
		}
		frac, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			t.Fatalf("FormatAmount(%d) = %q: cannot parse fractional part: %v", cents, out, err)
		}

		got := whole*100 + frac
		if got != cents {
			t.Fatalf("FormatAmount(%d) = %q parses back to %d, want %d", cents, out, got, cents)
		}

		// The parsed parts must match the expected decomposition.
		if whole != cents/100 || frac != cents%100 {
			t.Fatalf("FormatAmount(%d) = %q decomposes to whole=%d frac=%d, want whole=%d frac=%d",
				cents, out, whole, frac, cents/100, cents%100)
		}
	})
}
