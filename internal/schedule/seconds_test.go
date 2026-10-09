package schedule_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/schedule"
)

// A six-field expression is a five-field one with the seconds field written in
// front of it, which is what lets a job run more than once a minute.

func TestParseAcceptsASecondsField(t *testing.T) {
	expressions := []string{
		"* * * * * *",
		"*/5 * * * * *",
		"0 0 3 * * *",
		"30 30 2 * * *",
		"0,30 * * * * *",
		"10-20/5 * * * * *",
		"59 59 23 31 12 6",
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

func TestParseRejectsAnInvalidSecondsField(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{
			name:       "out of range",
			expression: "60 * * * * *",
			want:       "second field: 60 is out of range [0-59]",
		},
		{
			name:       "an unknown name",
			expression: "mon * * * * *",
			want:       `second field: "mon" is not a valid value`,
		},
		{
			name:       "a zero step",
			expression: "*/0 * * * * *",
			want:       "it must be at least 1",
		},
		{
			name:       "a non numeric step",
			expression: "*/x * * * * *",
			want:       "is not a number",
		},
		{
			name:       "a single value with a step",
			expression: "5/15 * * * * *",
			want:       `use "*/15" or a range`,
		},
		{
			name:       "an empty list item",
			expression: "1,,2 * * * * *",
			want:       "has an empty list item",
		},
		{
			name:       "a reversed range",
			expression: "5-1 * * * * *",
			want:       "is reversed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// EXERCISE
			_, err := schedule.Parse(tt.expression)

			// VERIFY
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, want an error", tt.expression)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse(%q) error = %q, want it to report %q", tt.expression, err, tt.want)
			}
		})
	}
}

func TestNextWithASecondsField(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		after      string // RFC 3339, UTC
		want       string // RFC 3339, UTC; empty means "no occurrence"
	}{
		{
			name:       "every five seconds",
			expression: "*/5 * * * * *",
			after:      "2026-01-01T10:00:03Z",
			want:       "2026-01-01T10:00:05Z",
		},
		{
			name:       "strictly after an activation",
			expression: "*/5 * * * * *",
			after:      "2026-01-01T10:00:05Z",
			want:       "2026-01-01T10:00:10Z",
		},
		{
			name:       "the last second of a minute rolls into the next minute",
			expression: "*/5 * * * * *",
			after:      "2026-01-01T10:00:55Z",
			want:       "2026-01-01T10:01:00Z",
		},
		{
			name:       "the next second, not the whole second",
			expression: "*/5 * * * * *",
			after:      "2026-01-01T10:00:03.5Z",
			want:       "2026-01-01T10:00:05Z",
		},
		{
			name:       "every second",
			expression: "* * * * * *",
			after:      "2026-01-01T10:00:00.5Z",
			want:       "2026-01-01T10:00:01Z",
		},
		{
			name:       "a second inside the minute",
			expression: "30 * * * * *",
			after:      "2026-01-01T10:00:00Z",
			want:       "2026-01-01T10:00:30Z",
		},
		{
			name:       "a second inside the minute, once it has passed",
			expression: "30 * * * * *",
			after:      "2026-01-01T10:00:30Z",
			want:       "2026-01-01T10:01:30Z",
		},
		{
			name:       "a list of seconds",
			expression: "0,30 * * * * *",
			after:      "2026-01-01T10:00:31Z",
			want:       "2026-01-01T10:01:00Z",
		},
		{
			name:       "a step inside a range restarts every minute",
			expression: "10-20/5 * * * * *",
			after:      "2026-01-01T10:00:20Z",
			want:       "2026-01-01T10:01:10Z",
		},
		{
			name:       "the rest of the fields still apply",
			expression: "15 30 2 * * *",
			after:      "2026-01-01T02:29:00Z",
			want:       "2026-01-01T02:30:15Z",
		},
		{
			name:       "a schedule with a seconds field that can never match",
			expression: "0 0 0 31 4 *", // April never has 31 days
			after:      "2026-01-01T00:00:00Z",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SETUP
			parsed := mustParse(t, tt.expression)
			after := mustTime(t, tt.after)

			// EXERCISE
			got, ok := parsed.Next(after)

			// VERIFY
			if tt.want == "" {
				if ok {
					t.Fatalf("Next(%s) = %s, want no occurrence", after, got)
				}
				return
			}
			want := mustTime(t, tt.want)
			if !ok {
				t.Fatalf("Next(%s) reported no occurrence, want %s", after, want)
			}
			if !got.Equal(want) {
				t.Fatalf("Next(%s) = %s, want %s", after, got, want)
			}
		})
	}
}

func TestParseRejectsMoreFieldsThanASecondsFieldAllows(t *testing.T) {
	// SETUP: the seconds field is the only optional one, so seven fields are
	// two too many.
	const expression = "* * * * * * *"

	// EXERCISE
	_, err := schedule.Parse(expression)

	// VERIFY
	if err == nil {
		t.Fatalf("Parse(%q) succeeded, want an error", expression)
	}
	for _, want := range []string{"must have 5 fields", "6 with the seconds field", "got 7"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q) error = %q, want it to report %q", expression, err, want)
		}
	}
}

func TestASecondsFieldOfZeroIsTheSameAsNoSecondsField(t *testing.T) {
	// SETUP: every six-field expression here states the seconds its five-field
	// twin leaves unsaid.
	pairs := []struct {
		fiveFields string
		sixFields  string
	}{
		{fiveFields: "* * * * *", sixFields: "0 * * * * *"},
		{fiveFields: "*/15 * * * *", sixFields: "0 */15 * * * *"},
		{fiveFields: "0 3 * * *", sixFields: "0 0 3 * * *"},
		{fiveFields: "30 2 * jan,jul *", sixFields: "0 30 2 * jan,jul *"},
		{fiveFields: "@daily", sixFields: "0 0 0 * * *"},
	}

	for _, pair := range pairs {
		t.Run(pair.sixFields, func(t *testing.T) {
			written := mustParse(t, pair.fiveFields)
			withSeconds := mustParse(t, pair.sixFields)

			// EXERCISE: the two are followed for a day of activations, which
			// is long enough for a difference to show.
			after := mustTime(t, "2026-01-01T00:00:00Z")
			for step := 0; step < 24; step++ {
				gotWritten, okWritten := written.Next(after)
				gotSeconds, okSeconds := withSeconds.Next(after)

				// VERIFY
				if okWritten != okSeconds {
					t.Fatalf("step %d: %q has an occurrence (%v) and %q does not (%v)",
						step, pair.fiveFields, okWritten, pair.sixFields, okSeconds)
				}
				if !okWritten {
					break
				}
				if !gotWritten.Equal(gotSeconds) {
					t.Fatalf("step %d: %q runs at %s and %q runs at %s",
						step, pair.fiveFields, gotWritten, pair.sixFields, gotSeconds)
				}
				if gotSeconds.Second() != 0 {
					t.Fatalf("step %d: %q runs at %s, want a whole minute",
						step, pair.sixFields, gotSeconds)
				}
				after = gotWritten
			}
		})
	}
}

func TestStringReturnsTheSecondsExpression(t *testing.T) {
	// SETUP
	const expression = "*/5 0 3 * * mon-fri"
	parsed := mustParse(t, expression)

	// EXERCISE
	got := parsed.String()

	// VERIFY
	if got != expression {
		t.Errorf("String() = %q, want %q", got, expression)
	}
}
