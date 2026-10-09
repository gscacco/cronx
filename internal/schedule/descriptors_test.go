package schedule_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/schedule"
)

// descriptorCases are the descriptors cronx accepts, each with the expression it
// stands for, exactly as docs/scheduling.md documents them.
var descriptorCases = []struct {
	descriptor string
	expression string
}{
	{descriptor: "@yearly", expression: "0 0 1 1 *"},
	{descriptor: "@monthly", expression: "0 0 1 * *"},
	{descriptor: "@weekly", expression: "0 0 * * 0"},
	{descriptor: "@daily", expression: "0 0 * * *"},
	{descriptor: "@midnight", expression: "0 0 * * *"},
	{descriptor: "@hourly", expression: "0 * * * *"},
}

// descriptorInstants are the instants a descriptor and the expression it stands
// for are compared from: an activation of the coarsest and of the finest of
// them, and three instants in between, so that every field is exercised.
var descriptorInstants = []string{
	"2026-01-01T00:00:00Z",
	"2026-01-01T00:01:00Z",
	"2026-06-15T12:34:00Z",
	"2026-10-09T23:59:00Z",
	"2026-12-31T23:30:00Z",
}

func TestParseAcceptsDescriptors(t *testing.T) {
	for _, tt := range descriptorCases {
		t.Run(tt.descriptor, func(t *testing.T) {
			// EXERCISE
			_, err := schedule.Parse(tt.descriptor)

			// VERIFY
			if err != nil {
				t.Fatalf("Parse(%q) returned an unexpected error: %v", tt.descriptor, err)
			}
		})
	}
}

func TestDescriptorsAreCaseInsensitive(t *testing.T) {
	for _, tt := range descriptorCases {
		for _, written := range []string{
			strings.ToUpper(tt.descriptor),
			strings.ToLower(tt.descriptor),
			"  " + strings.ToUpper(tt.descriptor) + "\t",
		} {
			t.Run(written, func(t *testing.T) {
				// EXERCISE
				parsed, err := schedule.Parse(written)

				// VERIFY
				if err != nil {
					t.Fatalf("Parse(%q) returned an unexpected error: %v", written, err)
				}
				if parsed.String() != written {
					t.Errorf("String() = %q, want %q: the descriptor is kept as it was written",
						parsed.String(), written)
				}
			})
		}
	}
}

func TestDescriptorsStandForTheDocumentedExpressions(t *testing.T) {
	for _, tt := range descriptorCases {
		t.Run(tt.descriptor, func(t *testing.T) {
			// SETUP
			descriptor := mustParse(t, tt.descriptor)
			expression := mustParse(t, tt.expression)

			for _, instant := range descriptorInstants {
				after := mustTime(t, instant)

				// EXERCISE
				got, gotFound := descriptor.Next(after)
				want, wantFound := expression.Next(after)

				// VERIFY
				if gotFound != wantFound || !got.Equal(want) {
					t.Errorf("%s: Next(%s) = %s (found %t), want %s (found %t), which is what %q gives",
						tt.descriptor, instant, got, gotFound, want, wantFound, tt.expression)
				}
			}
		})
	}
}

func TestParseRejectsInvalidDescriptors(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		want       string
	}{
		{name: "a misspelled descriptor", expression: "@dayly", want: "is not a known descriptor"},
		{name: "a descriptor cronx does not have", expression: "@reboot", want: "is not a known descriptor"},
		{name: "the marker on its own", expression: "@", want: "is not a known descriptor"},
		{name: "a descriptor followed by a field", expression: "@daily 0", want: "stands alone"},
		{name: "a descriptor followed by five fields", expression: "@hourly * * * *", want: "stands alone"},
		{name: "a descriptor followed by a day name", expression: "@weekly mon", want: "stands alone"},
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

// An unknown descriptor is answered with the list of the ones cronx accepts, so
// that a typo is enough to see what was meant.
func TestAnUnknownDescriptorIsAnsweredWithTheAcceptedOnes(t *testing.T) {
	// SETUP
	const expression = "@sometimes"

	// EXERCISE
	_, err := schedule.Parse(expression)

	// VERIFY
	if err == nil {
		t.Fatalf("Parse(%q) succeeded, want an error", expression)
	}
	for _, tt := range descriptorCases {
		if !strings.Contains(err.Error(), tt.descriptor) {
			t.Errorf("Parse(%q) error = %q, want it to name %q", expression, err, tt.descriptor)
		}
	}
}
