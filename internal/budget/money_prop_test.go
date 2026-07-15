package budget

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// validAmount pairs a generated valid amount string with the integer-cents
// value it is expected to parse to.
type validAmount struct {
	text  string
	cents int64
}

// genValidAmount generates amount strings in the range 0.01–999,999,999.99 with
// zero, one, or two decimal places, along with the exact cents they represent.
func genValidAmount() *rapid.Generator[validAmount] {
	return rapid.Custom(func(t *rapid.T) validAmount {
		decimals := rapid.IntRange(0, 2).Draw(t, "decimals")
		whole := rapid.Int64Range(0, maxAmountWhole).Draw(t, "whole")
		switch decimals {
		case 0:
			// No fractional digits, e.g. "12". Requires cents > 0.
			if whole == 0 {
				whole = 1
			}
			return validAmount{text: strconv.FormatInt(whole, 10), cents: whole * 100}
		case 1:
			// One fractional digit, e.g. "12.5" == 1250 cents.
			d1 := rapid.IntRange(0, 9).Draw(t, "d1")
			if whole == 0 && d1 == 0 {
				d1 = 1
			}
			return validAmount{
				text:  fmt.Sprintf("%d.%d", whole, d1),
				cents: whole*100 + int64(d1)*10,
			}
		default:
			// Two fractional digits, e.g. "12.50" or "0.01".
			frac := rapid.IntRange(0, 99).Draw(t, "frac")
			if whole == 0 && frac == 0 {
				frac = 1
			}
			return validAmount{
				text:  fmt.Sprintf("%d.%02d", whole, frac),
				cents: whole*100 + int64(frac),
			}
		}
	})
}

// genInvalidAmount generates amount strings that must be rejected: missing,
// non-numeric, less than or equal to zero, greater than the maximum, or with
// more than two decimal places.
func genInvalidAmount() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		kind := rapid.IntRange(0, 5).Draw(t, "kind")
		switch kind {
		case 0: // missing / blank
			return rapid.SampledFrom([]string{"", " ", "   ", "\t", "\n"}).Draw(t, "blank")
		case 1: // non-numeric (letters only)
			return rapid.StringMatching(`[a-zA-Z]{1,12}`).Draw(t, "letters")
		case 2: // less than or equal to zero
			return rapid.SampledFrom([]string{
				"0", "0.0", "0.00", "00", "000.00", "-1", "-0.01", "-1234.56",
			}).Draw(t, "nonpositive")
		case 3: // greater than 999,999,999.99
			whole := rapid.Int64Range(maxAmountWhole+1, 9_999_999_999_999).Draw(t, "big")
			if rapid.Bool().Draw(t, "withFrac") {
				return fmt.Sprintf("%d.%02d", whole, rapid.IntRange(0, 99).Draw(t, "bigFrac"))
			}
			return strconv.FormatInt(whole, 10)
		case 4: // more than two decimal places
			whole := rapid.Int64Range(0, maxAmountWhole).Draw(t, "wholeExtra")
			n := rapid.IntRange(3, 6).Draw(t, "numDecimals")
			var b strings.Builder
			for i := 0; i < n; i++ {
				b.WriteByte(byte('0' + rapid.IntRange(0, 9).Draw(t, "digit")))
			}
			return fmt.Sprintf("%d.%s", whole, b.String())
		default: // malformed numeric-looking input
			return rapid.SampledFrom([]string{
				"1.2.3", "12.", ".", ".5", "+5", "1e5", "1,000", "1 2", "0x10", "--1", "3.", "\u0663",
			}).Draw(t, "malformed")
		}
	})
}

// Feature: budget-tracker, Property 2: Amount validation rejects all invalid amounts
func TestParseAmountProp(t *testing.T) {
	// For any amount string in 0.01–999,999,999.99 with at most two decimal
	// places, ParseAmount accepts it and returns the correct integer cents.
	t.Run("accepts valid amounts", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			v := genValidAmount().Draw(t, "valid")
			cents, err := ParseAmount(v.text)
			if err != nil {
				t.Fatalf("ParseAmount(%q) returned error %v; want accepted", v.text, err)
			}
			if cents != v.cents {
				t.Fatalf("ParseAmount(%q) = %d cents; want %d", v.text, cents, v.cents)
			}
			if cents < 1 || cents > MaxAmountCents {
				t.Fatalf("ParseAmount(%q) = %d cents; outside valid range [1, %d]", v.text, cents, MaxAmountCents)
			}
		})
	})

	// For any amount string that is missing, non-numeric, <= 0, > the maximum,
	// or has more than two decimal places, ParseAmount rejects it with an error.
	t.Run("rejects invalid amounts", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			s := genInvalidAmount().Draw(t, "invalid")
			if cents, err := ParseAmount(s); err == nil {
				t.Fatalf("ParseAmount(%q) = %d cents with no error; want rejected", s, cents)
			}
		})
	})
}
