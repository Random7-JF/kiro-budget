package budget

import "testing"

// findFieldError returns the first FieldError whose Field matches the given
// name, along with whether such an error was present.
func findFieldError(errs []FieldError, field string) (FieldError, bool) {
	for _, e := range errs {
		if e.Field == field {
			return e, true
		}
	}
	return FieldError{}, false
}

// TestValidateMissingFieldMessages verifies that Validate reports a named,
// non-empty validation message for each missing required field (amount, date,
// and category), reinforcing the validation properties with explicit examples.
//
// Validates: Requirements 1.3, 1.4, 2.3, 2.5, 3.3
func TestValidateMissingFieldMessages(t *testing.T) {
	// A baseline of otherwise-valid field values so each case isolates a
	// single missing field.
	const (
		validAmount   = "12.34"
		validDate     = "2024-05-13"
		validCategory = "Groceries"
	)

	cases := []struct {
		name      string
		input     TransactionInput
		wantField string
	}{
		{
			name: "missing amount",
			input: TransactionInput{
				Type:       TypeExpense,
				AmountText: "",
				DateText:   validDate,
				Category:   validCategory,
			},
			wantField: "amount",
		},
		{
			name: "missing date",
			input: TransactionInput{
				Type:       TypeExpense,
				AmountText: validAmount,
				DateText:   "",
				Category:   validCategory,
			},
			wantField: "date",
		},
		{
			name: "missing category",
			input: TransactionInput{
				Type:       TypeExpense,
				AmountText: validAmount,
				DateText:   validDate,
				Category:   "",
			},
			wantField: "category",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, errs := Validate(tc.input)

			fe, ok := findFieldError(errs, tc.wantField)
			if !ok {
				t.Fatalf("Validate(%+v) = errors %+v; want a FieldError with Field %q",
					tc.input, errs, tc.wantField)
			}
			if fe.Message == "" {
				t.Errorf("FieldError for %q has empty Message; want a non-empty message", tc.wantField)
			}

			// A rejected entry must not produce a normalized transaction.
			if got != (Transaction{}) {
				t.Errorf("Validate(%+v) = %+v; want zero Transaction on rejection", tc.input, got)
			}
		})
	}
}

// TestValidateFullyMissingInput verifies that an input missing all three
// required fields yields a FieldError for each of amount, date, and category
// (each with a non-empty message) and a zero Transaction.
//
// Validates: Requirements 1.3, 1.4, 2.3, 2.5, 3.3
func TestValidateFullyMissingInput(t *testing.T) {
	input := TransactionInput{Type: TypeExpense}

	got, errs := Validate(input)

	for _, field := range []string{"amount", "date", "category"} {
		fe, ok := findFieldError(errs, field)
		if !ok {
			t.Errorf("Validate(%+v) errors %+v: missing FieldError for %q", input, errs, field)
			continue
		}
		if fe.Message == "" {
			t.Errorf("FieldError for %q has empty Message; want a non-empty message", field)
		}
	}

	if got != (Transaction{}) {
		t.Errorf("Validate(%+v) = %+v; want zero Transaction on rejection", input, got)
	}
}
