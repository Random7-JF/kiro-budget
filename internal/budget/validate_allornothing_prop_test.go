package budget

import (
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// aonField pairs a generated field value with whether it is valid. When
// invalid, the field's canonical name (matching validate.go: "amount",
// "date", "category") is expected to appear in the returned error set.
type aonField struct {
	text  string
	valid bool
}

// genAONAmount independently generates an amount string that is either valid
// (e.g. "12.34") or invalid in one of the ways ParseAmount rejects (<= 0,
// non-numeric, empty, > max, or > 2 decimal places).
func genAONAmount() *rapid.Generator[aonField] {
	return rapid.Custom(func(t *rapid.T) aonField {
		if rapid.Bool().Draw(t, "amountValid") {
			text := rapid.SampledFrom([]string{
				"12.34", "1", "0.01", "999999999.99", "500.5", "42", "1000.00",
			}).Draw(t, "validAmount")
			return aonField{text: text, valid: true}
		}
		text := rapid.SampledFrom([]string{
			"0", "0.00", "-1", "-0.01", "", "   ", "abc", "1.2.3", "12.",
			"1000000000", "1000000000.00", "1.234", "12.999",
		}).Draw(t, "invalidAmount")
		return aonField{text: text, valid: false}
	})
}

// genAONDate independently generates a date string that is either a valid ISO
// 8601 calendar date or invalid (empty or an impossible/malformed date).
func genAONDate() *rapid.Generator[aonField] {
	return rapid.Custom(func(t *rapid.T) aonField {
		if rapid.Bool().Draw(t, "dateValid") {
			text := rapid.SampledFrom([]string{
				"2024-01-15", "2000-02-29", "1999-12-31", "2024-06-30", "2023-03-01",
			}).Draw(t, "validDate")
			return aonField{text: text, valid: true}
		}
		text := rapid.SampledFrom([]string{
			"", "2024-13-40", "2024-02-30", "2024-00-10", "15-01-2024",
			"2024/01/15", "not-a-date", "2023-13-01",
		}).Draw(t, "invalidDate")
		return aonField{text: text, valid: false}
	})
}

// genAONCategory independently generates a category that is either valid
// (1-100 chars) or invalid (empty or over 100 chars).
func genAONCategory() *rapid.Generator[aonField] {
	return rapid.Custom(func(t *rapid.T) aonField {
		if rapid.Bool().Draw(t, "categoryValid") {
			text := rapid.SampledFrom([]string{
				"Food", "Rent", "Salary", "Groceries", "A",
			}).Draw(t, "validCategory")
			return aonField{text: text, valid: true}
		}
		if rapid.Bool().Draw(t, "emptyCategory") {
			return aonField{text: "", valid: false}
		}
		// Over 100 characters.
		n := rapid.IntRange(101, 200).Draw(t, "catLen")
		return aonField{text: strings.Repeat("x", n), valid: false}
	})
}

// Feature: budget-tracker, Property 5: Edit with any invalid field is an all-or-nothing rejection
func TestValidateAllOrNothingProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		amount := genAONAmount().Draw(t, "amount")
		date := genAONDate().Draw(t, "date")
		category := genAONCategory().Draw(t, "category")

		// Build the expected set of invalid field names.
		var expected []string
		if !amount.valid {
			expected = append(expected, "amount")
		}
		if !date.valid {
			expected = append(expected, "date")
		}
		if !category.valid {
			expected = append(expected, "category")
		}

		input := TransactionInput{
			ID:          7,
			Type:        TypeExpense,
			AmountText:  amount.text,
			DateText:    date.text,
			Category:    category.text,
			Description: "an edit",
		}

		txn, errs := Validate(input)

		if len(expected) == 0 {
			// All fields valid: no errors and a normalized transaction.
			if len(errs) != 0 {
				t.Fatalf("all fields valid but got errors %+v (input=%+v)", errs, input)
			}
			if (txn == Transaction{}) {
				t.Fatalf("all fields valid but got zero Transaction (input=%+v)", input)
			}
			return
		}

		// One or more invalid fields: all-or-nothing rejection. No normalized
		// transaction is produced.
		if (txn != Transaction{}) {
			t.Fatalf("invalid input produced a non-zero Transaction %+v (input=%+v)", txn, input)
		}

		// The returned errors must identify EXACTLY the set of invalid fields:
		// no more, no fewer, and no duplicates.
		got := make([]string, 0, len(errs))
		seen := make(map[string]bool)
		for _, fe := range errs {
			if seen[fe.Field] {
				t.Fatalf("duplicate error for field %q in %+v", fe.Field, errs)
			}
			seen[fe.Field] = true
			got = append(got, fe.Field)
		}

		if !equalStringSets(got, expected) {
			t.Fatalf("error fields = %v, want exactly %v (input=%+v)", sortedCopy(got), sortedCopy(expected), input)
		}
	})
}

// equalStringSets reports whether a and b contain exactly the same elements,
// treated as sets (order-independent, assuming a has no duplicates).
func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := sortedCopy(a)
	sb := sortedCopy(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// sortedCopy returns a sorted copy of s for stable comparison and reporting.
func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
