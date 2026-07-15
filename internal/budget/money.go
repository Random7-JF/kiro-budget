package budget

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxAmountCents is the largest valid amount magnitude in integer cents,
// corresponding to 999,999,999.99.
const MaxAmountCents int64 = 99999999999

// maxAmountWhole is the largest valid whole-currency part (999,999,999).
const maxAmountWhole int64 = 999999999

// ParseAmount parses a decimal money string into a non-negative integer count
// of cents.
//
// It accepts values from 0.01 to 999,999,999.99 with at most two decimal
// places (for example "12", "12.5", "12.50", "0.01", "999999999.99"). It
// rejects input that is missing, non-numeric, less than or equal to zero,
// greater than 999,999,999.99, or has more than two decimal places.
func ParseAmount(text string) (cents int64, err error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return 0, errors.New("amount is required")
	}

	intStr := s
	fracStr := ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intStr = s[:dot]
		fracStr = s[dot+1:]
		// A second decimal point (or any non-digit) makes it non-numeric.
		if strings.IndexByte(fracStr, '.') >= 0 {
			return 0, errors.New("amount is not a valid number")
		}
		if len(fracStr) == 0 {
			// Trailing decimal point with no fractional digits, e.g. "12.".
			return 0, errors.New("amount is not a valid number")
		}
		if len(fracStr) > 2 {
			return 0, errors.New("amount must have at most two decimal places")
		}
	}

	// Require an integer part and only decimal digits in both parts. This
	// rejects signs ("+"/"-"), exponents ("1e5"), spaces, and other symbols.
	if intStr == "" || !allDigits(intStr) || !allDigits(fracStr) {
		return 0, errors.New("amount is not a valid number")
	}

	whole, convErr := strconv.ParseInt(intStr, 10, 64)
	if convErr != nil || whole > maxAmountWhole {
		return 0, errors.New("amount must not exceed 999,999,999.99")
	}

	frac := int64(0)
	if fracStr != "" {
		padded := fracStr
		if len(padded) == 1 {
			padded += "0" // "5" represents fifty cents, not five.
		}
		// padded is exactly two digits of guaranteed-numeric text.
		frac, _ = strconv.ParseInt(padded, 10, 64)
	}

	cents = whole*100 + frac
	if cents <= 0 {
		return 0, errors.New("amount must be greater than zero")
	}
	if cents > MaxAmountCents {
		return 0, errors.New("amount must not exceed 999,999,999.99")
	}
	return cents, nil
}

// FormatAmount renders an integer count of cents as a fixed two-decimal string
// (for example 1250 becomes "12.50"). Negative values (which can arise for a
// net balance) are rendered with a leading minus sign.
func FormatAmount(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	whole := cents / 100
	frac := cents % 100
	s := fmt.Sprintf("%d.%02d", whole, frac)
	if negative {
		s = "-" + s
	}
	return s
}

// allDigits reports whether s consists solely of ASCII decimal digits. The
// empty string is considered all-digits (vacuously true); callers guard the
// integer part against emptiness separately.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
