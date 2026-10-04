package schedule_test

import (
	"testing"

	"gscacco.com/cronx/internal/schedule"
)

func TestParseAcceptsValidExpressions(t *testing.T) {
	expressions := []string{
		"* * * * *",
		"0 3 * * *",
		"*/15 * * * *",
		"0 */6 * * *",
		"0 0-6/2 * * *",
		"5,10,15 * * * *",
		"0 0 1,15 * *",
		"0 12 * jan-mar *",
		"0 12 * * mon-fri",
		"0 0 * * 7",
		"0 0 29 2 *",
		"59 23 31 12 6",
		"0 0 1 JAN *",
	}

	for _, expression := range expressions {
		t.Run(expression, func(t *testing.T) {
			// EXERCISE
			_, err := schedule.Parse(expression)

			// VERIFY
			if err != nil {
				t.Fatalf("Parse(%q) returned an unexpected error: %v", expression, err)
			}
		})
	}
}

func TestParseRejectsInvalidExpressions(t *testing.T) {
	tests := []struct {
		name       string
		expression string
	}{
		{"empty", ""},
		{"too few fields", "* * * *"},
		{"too many fields", "* * * * * *"},
		{"minute out of range", "60 * * * *"},
		{"hour out of range", "* 24 * * *"},
		{"day of month zero", "* * 0 * *"},
		{"day of month out of range", "* * 32 * *"},
		{"month out of range", "* * * 13 *"},
		{"day of week out of range", "* * * * 8"},
		{"reversed range", "5-1 * * * *"},
		{"zero step", "*/0 * * * *"},
		{"non numeric step", "*/x * * * *"},
		{"non numeric value", "a * * * *"},
		{"unknown month name", "0 12 * january *"},
		{"unknown day name", "0 0 * * someday"},
		{"descriptor", "@daily"},
		{"range with a missing end", "* * * * mon-"},
		{"range with a missing start", "* * * * -fri"},
		{"empty list item", "1,,2 * * * *"},
		{"a single value cannot carry a step", "5/15 * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// EXERCISE
			_, err := schedule.Parse(tt.expression)

			// VERIFY
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error", tt.expression)
			}
		})
	}
}

func TestStringReturnsTheOriginalExpression(t *testing.T) {
	// SETUP
	const expression = "*/15 0 1,15 jan-mar mon-fri"
	parsed, err := schedule.Parse(expression)
	if err != nil {
		t.Fatalf("Parse(%q) returned an unexpected error: %v", expression, err)
	}

	// EXERCISE
	got := parsed.String()

	// VERIFY
	if got != expression {
		t.Errorf("String() = %q, want %q", got, expression)
	}
}
