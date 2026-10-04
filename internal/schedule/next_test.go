package schedule_test

import (
	"testing"
	"time"

	"gscacco.com/cronx/internal/schedule"
)

func mustParse(t *testing.T, expression string) schedule.Schedule {
	t.Helper()
	parsed, err := schedule.Parse(expression)
	if err != nil {
		t.Fatalf("Parse(%q) returned an unexpected error: %v", expression, err)
	}
	return parsed
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parsing time %q: %v", value, err)
	}
	return parsed
}

func TestNext(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		after      string // RFC 3339, UTC
		want       string // RFC 3339, UTC; empty means "no occurrence"
	}{
		{
			name:       "every minute",
			expression: "* * * * *",
			after:      "2026-01-01T10:00:00Z",
			want:       "2026-01-01T10:01:00Z",
		},
		{
			name:       "daily at 03:00",
			expression: "0 3 * * *",
			after:      "2026-01-01T02:59:00Z",
			want:       "2026-01-01T03:00:00Z",
		},
		{
			name:       "strictly after an exact match",
			expression: "0 3 * * *",
			after:      "2026-01-01T03:00:00Z",
			want:       "2026-01-02T03:00:00Z",
		},
		{
			name:       "every 15 minutes",
			expression: "*/15 * * * *",
			after:      "2026-01-01T10:07:00Z",
			want:       "2026-01-01T10:15:00Z",
		},
		{
			name:       "every 6 hours",
			expression: "0 */6 * * *",
			after:      "2026-01-01T05:00:00Z",
			want:       "2026-01-01T06:00:00Z",
		},
		{
			name:       "hour range with a step",
			expression: "0 0-6/2 * * *",
			after:      "2026-01-01T05:00:00Z",
			want:       "2026-01-01T06:00:00Z",
		},
		{
			name:       "hour range end rolls into the next day",
			expression: "0 0-6/2 * * *",
			after:      "2026-01-01T06:00:00Z",
			want:       "2026-01-02T00:00:00Z",
		},
		{
			name:       "weekday by number (Monday)",
			expression: "0 0 * * 1",
			after:      "2026-01-01T00:00:00Z", // a Thursday
			want:       "2026-01-05T00:00:00Z",
		},
		{
			name:       "weekday by name",
			expression: "0 0 * * mon",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-05T00:00:00Z",
		},
		{
			name:       "sunday is both 0 and 7",
			expression: "0 0 * * 7",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-04T00:00:00Z",
		},
		{
			name:       "first day of the month",
			expression: "0 0 1 * *",
			after:      "2026-01-15T00:00:00Z",
			want:       "2026-02-01T00:00:00Z",
		},
		{
			name:       "month by name",
			expression: "0 12 * jan *",
			after:      "2026-03-01T00:00:00Z",
			want:       "2027-01-01T12:00:00Z",
		},
		{
			name:       "day of month and day of week are OR-ed (day of week first)",
			expression: "0 0 13 * fri",
			after:      "2026-01-01T00:00:00Z",
			want:       "2026-01-02T00:00:00Z",
		},
		{
			name:       "day of month and day of week are OR-ed (day of month first)",
			expression: "0 0 13 * fri",
			after:      "2026-01-09T00:00:00Z",
			want:       "2026-01-13T00:00:00Z",
		},
		{
			name:       "leap day",
			expression: "0 0 29 2 *",
			after:      "2026-03-01T00:00:00Z",
			want:       "2028-02-29T00:00:00Z",
		},
		{
			name:       "leap day across a skipped century leap year",
			expression: "0 0 29 2 *",
			after:      "2096-03-01T00:00:00Z", // 2100 is not a leap year
			want:       "2104-02-29T00:00:00Z",
		},
		{
			name:       "impossible schedule has no occurrence",
			expression: "0 0 31 4 *", // April never has 31 days
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

func TestNextKeepsTheReferenceLocation(t *testing.T) {
	// SETUP
	parsed := mustParse(t, "0 12 * * *")
	zone := time.FixedZone("UTC+5", 5*60*60)
	after := time.Date(2026, 1, 1, 12, 0, 0, 0, zone)
	want := time.Date(2026, 1, 2, 12, 0, 0, 0, zone)

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
