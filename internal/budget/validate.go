package budget

import (
	"time"
	"unicode/utf8"
)

// dateLayout is the ISO 8601 calendar-date layout (YYYY-MM-DD). Parsing with
// this layout rejects impossible calendar dates such as "2024-02-30" or
// "2024-13-01".
const dateLayout = "2006-01-02"

// maxCategoryLen is the maximum number of characters allowed in a category.
const maxCategoryLen = 100

// Validate checks a raw TransactionInput for a create or edit operation and
// returns either a normalized Transaction (when every field is valid) or the
// complete set of per-field validation errors.
//
// Validation is all-or-nothing: if any field is invalid, Validate returns the
// zero Transaction along with a FieldError for every invalid field. Only when
// amount, date, and category are all valid does it return the normalized
// Transaction (with a nil error slice), preserving the input's ID, Type, and
// Description.
//
//   - amount is parsed via ParseAmount; failures yield a FieldError with
//     Field "amount".
//   - date must be a valid ISO 8601 calendar date in YYYY-MM-DD format, parsed
//     strictly with time.Parse; failures yield a FieldError with Field "date".
//   - category must be a non-empty label of 1 to 100 characters; failures yield
//     a FieldError with Field "category".
func Validate(input TransactionInput) (Transaction, []FieldError) {
	var errs []FieldError

	cents, amountErr := ParseAmount(input.AmountText)
	if amountErr != nil {
		errs = append(errs, FieldError{Field: "amount", Message: amountErr.Error()})
	}

	date, dateErr := time.Parse(dateLayout, input.DateText)
	if dateErr != nil {
		message := "date must be a valid calendar date in YYYY-MM-DD format"
		if input.DateText == "" {
			message = "date is required"
		}
		errs = append(errs, FieldError{Field: "date", Message: message})
	}

	if catErr := validateCategory(input.Category); catErr != "" {
		errs = append(errs, FieldError{Field: "category", Message: catErr})
	}

	if len(errs) > 0 {
		return Transaction{}, errs
	}

	return Transaction{
		ID:          input.ID,
		Type:        input.Type,
		AmountCents: cents,
		Date:        date,
		Category:    input.Category,
		Description: input.Description,
	}, nil
}

// validateCategory reports a validation message for an invalid category, or the
// empty string when the category is valid. A valid category is a non-empty
// label of 1 to 100 characters.
func validateCategory(category string) string {
	if category == "" {
		return "category is required"
	}
	if utf8.RuneCountInString(category) > maxCategoryLen {
		return "category must be at most 100 characters"
	}
	return ""
}
