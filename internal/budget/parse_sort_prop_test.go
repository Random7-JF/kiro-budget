package budget

import (
	"testing"

	"pgregory.net/rapid"
)

// genSortFieldParam draws a raw sort-field query-parameter string, mixing the
// recognized values ("date", "amount") with unrecognized ones (including the
// empty/absent value and arbitrary noise) so both the selection and fallback
// paths are exercised.
func genSortFieldParam(t *rapid.T) string {
	recognized := []string{"date", "amount"}
	unrecognized := []string{"", "DATE", "Amount", "id", "category", "amt", " date ", "descending", "date "}
	if rapid.Bool().Draw(t, "fieldRecognized") {
		return rapid.SampledFrom(recognized).Draw(t, "fieldKnown")
	}
	if rapid.Bool().Draw(t, "fieldFromPool") {
		return rapid.SampledFrom(unrecognized).Draw(t, "fieldPool")
	}
	return rapid.String().Draw(t, "fieldRandom")
}

// genSortDirParam draws a raw sort-direction query-parameter string, mixing the
// recognized values ("asc", "desc") with unrecognized ones (including the
// empty/absent value and arbitrary noise).
func genSortDirParam(t *rapid.T) string {
	recognized := []string{"asc", "desc"}
	unrecognized := []string{"", "ASC", "Desc", "ascending", "descending", "up", "down", "asc ", "0"}
	if rapid.Bool().Draw(t, "dirRecognized") {
		return rapid.SampledFrom(recognized).Draw(t, "dirKnown")
	}
	if rapid.Bool().Draw(t, "dirFromPool") {
		return rapid.SampledFrom(unrecognized).Draw(t, "dirPool")
	}
	return rapid.String().Draw(t, "dirRandom")
}

// isRecognizedSortField reports whether s is exactly one of the recognized sort
// field values.
func isRecognizedSortField(s string) bool {
	return SortField(s) == SortByDate || SortField(s) == SortByAmount
}

// isRecognizedSortDir reports whether s is exactly one of the recognized sort
// direction values.
func isRecognizedSortDir(s string) bool {
	return SortDir(s) == SortAsc || SortDir(s) == SortDesc
}

// Feature: budget-tracker, Property 18: Sort parsing defaults to date descending
func TestParseSortProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		field := genSortFieldParam(t)
		dir := genSortDirParam(t)

		gotField, gotDir := ParseSort(field, dir)

		if isRecognizedSortField(field) && isRecognizedSortDir(dir) {
			// Both recognized: parsing must return exactly that selection.
			if gotField != SortField(field) || gotDir != SortDir(dir) {
				t.Fatalf("ParseSort(%q, %q) = (%q, %q), want (%q, %q)",
					field, dir, gotField, gotDir, field, dir)
			}
			return
		}

		// Either value absent/unrecognized: parsing must fall back to the
		// default ordering of Transaction_Date descending.
		if gotField != SortByDate || gotDir != SortDesc {
			t.Fatalf("ParseSort(%q, %q) = (%q, %q), want fallback (%q, %q)",
				field, dir, gotField, gotDir, SortByDate, SortDesc)
		}
	})
}
