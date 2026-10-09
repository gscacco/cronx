package schedule_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/schedule"
)

// The L, W and # operators name days a plain value cannot: the last day of a
// month, the weekday nearest to one, the last occurrence of a weekday in a month
// and the n-th occurrence of one.

func TestParseAcceptsDayOperators(t *testing.T) {
	expressions := []string{
		"0 0 L * *",       // the last day of the month
		"0 0 LW * *",      // the last weekday of the month
		"0 0 1W * *",      // the weekday nearest to the first
		"0 0 15W * *",     // the weekday nearest to the fifteenth
		"0 0 31W * *",     // the weekday nearest to the thirty-first
		"0 0 1,15,L * *",  // a list of plain days and an operator
		"0 0 LW,15W * *",  // a list of two operators
		"0 0 * * L",       // the last day of the week
		"0 0 * * 5L",      // the last Friday of the month
		"0 0 * * friL",    // the same, by name
		"0 0 * * 7L",      // the last Sunday of the month, as 7
		"0 0 * * 5#2",     // the second Friday of the month
		"0 0 * * fri#2",   // the same, by name
		"0 0 * * sun#1",   // the first Sunday of the month
		"0 0 * * mon,5#2", // a list of a plain weekday and an operator
		"0 0 * * 0,5#2,6L",
		"0 0 L * 5#2",   // both day fields, which are OR-ed
		"*/5 0 0 L * *", // with a seconds field in front
		"0 0 L JUL WED", // the operators do not disturb the names
		"0 0 * * SAT#4", // the names are case-insensitive
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

func TestParseRejectsInvalidDayOperators(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string // a substring of the message
	}{
		{
			name:       "a step on the last day of the month",
			expression: "0 0 L/2 * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "a range that ends at the last day of the month",
			expression: "0 0 1-L * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "the last day of the month with a day after it",
			expression: "0 0 L2 * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "a nearest weekday without a day",
			expression: "0 0 W * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "a nearest weekday that is not a number",
			expression: "0 0 xW * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "a nearest weekday beyond the month",
			expression: "0 0 32W * *",
			want:       "32 is out of range",
		},
		{
			name:       "the zeroth day of a nearest weekday",
			expression: "0 0 0W * *",
			want:       "0 is out of range",
		},
		{
			name:       "the last-of operator in the day of month",
			expression: "0 0 5L * *",
			want:       `use "L" for the last day of the month`,
		},
		{
			name:       "the occurrence operator in the day of month",
			expression: "0 0 5#2 * *",
			want:       "belongs to the day of week field",
		},
		{
			name:       "the nearest-weekday operator in the day of week",
			expression: "0 0 * * 5W",
			want:       "belongs to the day of month field",
		},
		{
			name:       "the last weekday of the month in the day of week",
			expression: "0 0 * * LW",
			want:       "belongs to the day of month field",
		},
		{
			name:       "the bare nearest-weekday operator in the day of week",
			expression: "0 0 * * W",
			want:       "belongs to the day of month field",
		},
		{
			name:       "a step on the last Friday",
			expression: "0 0 * * 5L/2",
			want:       `use "L" for the last day of the week`,
		},
		{
			name:       "a range of last Fridays",
			expression: "0 0 * * 4L-5L",
			want:       `"4L-5" is not a valid value`,
		},
		{
			name:       "a weekday that does not exist before the last-of operator",
			expression: "0 0 * * somedayL",
			want:       `"someday" is not a valid value`,
		},
		{
			name:       "an occurrence beyond the fifth",
			expression: "0 0 * * 5#6",
			want:       "between one and five occurrences",
		},
		{
			name:       "the zeroth occurrence",
			expression: "0 0 * * 5#0",
			want:       "between one and five occurrences",
		},
		{
			name:       "an occurrence that is not a number",
			expression: "0 0 * * 5#fifth",
			want:       "is not the number of an occurrence",
		},
		{
			name:       "a missing occurrence",
			expression: "0 0 * * 5#",
			want:       "is not the number of an occurrence",
		},
		{
			name:       "a weekday that does not exist before the occurrence",
			expression: "0 0 * * someday#2",
			want:       `"someday" is not a valid value`,
		},
		{
			name:       "the last-of operator in the minute field",
			expression: "L * * * *",
			want:       "belong to the day of month and day of week fields",
		},
		{
			name:       "the nearest-weekday operator in the hour field",
			expression: "0 W * * *",
			want:       "belong to the day of month and day of week fields",
		},
		{
			name:       "the occurrence operator in the month field",
			expression: "0 0 * # *",
			want:       "belong to the day of month and day of week fields",
		},
		{
			name:       "the last-of operator in the seconds field",
			expression: "L * * * * *",
			want:       "belong to the day of month and day of week fields",
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
				t.Errorf("Parse(%q) error = %q, want it to mention %q", tt.expression, err, tt.want)
			}
		})
	}
}

func TestNextWithDayOperators(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		after      string // RFC 3339, UTC
		want       string // RFC 3339, UTC; empty means "no occurrence"
	}{
		{
			name:       "the last day of a month of thirty-one days",
			expression: "0 0 L * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-31T00:00:00Z",
		},
		{
			name:       "the last day of a month of twenty-eight days",
			expression: "0 0 L * *",
			after:      "2026-01-31T00:00:00Z",
			want:       "2026-02-28T00:00:00Z",
		},
		{
			name:       "the last day of a named month",
			expression: "0 0 L 12 *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-12-31T00:00:00Z",
		},
		{
			name:       "the last day of the following year",
			expression: "0 0 L 12 *",
			after:      "2026-12-31T00:00:00Z",
			want:       "2027-12-31T00:00:00Z",
		},
		{
			name:       "the last weekday when the month ends on a Saturday",
			expression: "0 0 LW * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-30T00:00:00Z",
		},
		{
			name:       "the last weekday when the month ends on a weekday",
			expression: "0 0 LW * *",
			after:      "2026-02-01T00:00:00Z",
			want:       "2026-02-27T00:00:00Z",
		},
		{
			name:       "the last weekday when the month ends on a Tuesday",
			expression: "0 0 LW * *",
			after:      "2026-03-01T00:00:00Z",
			want:       "2026-03-31T00:00:00Z",
		},
		{
			name:       "the weekday nearest to a day that is one",
			expression: "0 0 15W * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-15T00:00:00Z",
		},
		{
			name:       "the weekday nearest to a Saturday",
			expression: "0 0 10W * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-09T00:00:00Z",
		},
		{
			name:       "the weekday nearest to a Sunday",
			expression: "0 0 11W * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-12T00:00:00Z",
		},
		{
			name:       "the weekday nearest to a first that is a Saturday",
			expression: "0 0 1W 8 *",
			after:      "2026-07-31T00:00:00Z",
			want:       "2026-08-03T00:00:00Z",
		},
		{
			name:       "the weekday nearest to a last that is a Sunday",
			expression: "0 0 28W 2 *",
			after:      "2027-02-01T00:00:00Z",
			want:       "2027-02-26T00:00:00Z",
		},
		{
			name:       "a nearest weekday of a month that holds the day",
			expression: "0 0 31W * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-30T00:00:00Z",
		},
		{
			name:       "a nearest weekday skips the months that do not hold the day",
			expression: "0 0 31W * *",
			after:      "2026-01-30T00:00:00Z",
			want:       "2026-03-31T00:00:00Z",
		},
		{
			name:       "the last Friday of the month",
			expression: "0 0 * * 5L",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-30T00:00:00Z",
		},
		{
			name:       "the last Friday of the following month",
			expression: "0 0 * * 5L",
			after:      "2026-01-30T00:00:00Z",
			want:       "2026-02-27T00:00:00Z",
		},
		{
			name:       "the last Friday by name",
			expression: "0 0 * * friL",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-30T00:00:00Z",
		},
		{
			name:       "the last day of the week",
			expression: "0 0 * * L",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-03T00:00:00Z",
		},
		{
			name:       "the second Friday of the month",
			expression: "0 0 * * 5#2",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-09T00:00:00Z",
		},
		{
			name:       "the second Friday of the following month",
			expression: "0 0 * * 5#2",
			after:      "2026-01-09T00:00:00Z",
			want:       "2026-02-13T00:00:00Z",
		},
		{
			name:       "the first Sunday of the month",
			expression: "0 0 * * 0#1",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-04T00:00:00Z",
		},
		{
			name:       "the fifth Saturday of a month that holds one",
			expression: "0 0 * * 6#5",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-31T00:00:00Z",
		},
		{
			name:       "a fifth Saturday skips the months that hold four",
			expression: "0 0 * * 6#5",
			after:      "2026-01-31T00:00:00Z",
			want:       "2026-05-30T00:00:00Z",
		},
		{
			name:       "an operator and a plain day of week are OR-ed",
			expression: "0 0 L * mon",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-05T00:00:00Z",
		},
		{
			name:       "an operator and a plain day of week are OR-ed again",
			expression: "0 0 L * mon",
			after:      "2026-01-05T00:00:00Z",
			want:       "2026-01-12T00:00:00Z",
		},
		{
			name:       "both day fields carry an operator",
			expression: "0 0 L * 5#2",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-09T00:00:00Z",
		},
		{
			name:       "both day fields carry an operator and the last day comes last",
			expression: "0 0 L * 5#2",
			after:      "2026-01-09T00:00:00Z",
			want:       "2026-01-31T00:00:00Z",
		},
		{
			name:       "a list of plain days and an operator",
			expression: "0 0 1,15,L * *",
			after:      "2026-01-02T00:00:00Z",
			want:       "2026-01-15T00:00:00Z",
		},
		{
			name:       "a list of plain days and an operator at its end",
			expression: "0 0 1,15,L * *",
			after:      "2026-01-15T00:00:00Z",
			want:       "2026-01-31T00:00:00Z",
		},
		{
			name:       "an operator with a seconds field in front",
			expression: "10 0 0 L * *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-31T00:00:10Z",
		},
		{
			name:       "an operator in the day of month beside a restricted month",
			expression: "0 0 LW 2 *",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-02-27T00:00:00Z",
		},
		{
			name:       "a nearest weekday of a day no February holds",
			expression: "0 0 31W 2 *",
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

func TestNextWithTheLastDayOfTheWeekOperatorIsSaturday(t *testing.T) {
	// SETUP
	lastOfWeek := mustParse(t, "0 0 * * L")
	saturday := mustParse(t, "0 0 * * sat")
	after := mustTime(t, "2026-01-01T00:00:00Z")

	// EXERCISE, VERIFY: forty activations, one week apart.
	for step := 1; step <= 40; step++ {
		gotLast, okLast := lastOfWeek.Next(after)
		gotSaturday, okSaturday := saturday.Next(after)
		if !okLast || !okSaturday {
			t.Fatalf("step %d: Next(%s) reported no occurrence for L or for sat", step, after)
		}
		if !gotLast.Equal(gotSaturday) {
			t.Fatalf("step %d: Next(%s) = %s for L and %s for sat, want the same instant",
				step, after, gotLast, gotSaturday)
		}
		after = gotLast
	}
}

func TestNextWithDayOperatorsKeepsTheReferenceLocation(t *testing.T) {
	// SETUP
	parsed := mustParse(t, "0 0 L * *")
	zone := time.FixedZone("UTC+5", 5*60*60)
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, zone)
	want := time.Date(2026, 1, 31, 0, 0, 0, 0, zone)

	// EXERCISE
	got, ok := parsed.Next(after)

	// VERIFY
	if !ok {
		t.Fatalf("Next(%s) reported no occurrence, want %s", after, want)
	}
	if !got.Equal(want) {
		t.Fatalf("Next(%s) = %s, want %s", after, got, want)
	}
	if got.Location().String() != zone.String() {
		t.Errorf("Next(%s) location = %s, want %s", after, got.Location(), zone)
	}
}

func TestStringKeepsTheDayOperators(t *testing.T) {
	const expression = "0 0 L * fri#2"

	// EXERCISE
	parsed := mustParse(t, expression)

	// VERIFY
	if got := parsed.String(); got != expression {
		t.Errorf("String() = %q, want %q", got, expression)
	}
}
