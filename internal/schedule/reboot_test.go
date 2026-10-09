package schedule_test

import (
	"strings"
	"testing"

	"gscacco.com/cronx/internal/schedule"
)

// The expressions that are triggered by the clock, which are all of them but
// "@reboot".
var clockExpressions = []string{
	"@yearly", "@monthly", "@weekly", "@daily", "@midnight", "@hourly",
	"* * * * *", "0 3 * * *", "*/15 * * * *",
}

func TestParseAcceptsReboot(t *testing.T) {
	for _, written := range []string{"@reboot", "@REBOOT", "  @Reboot\t"} {
		t.Run(written, func(t *testing.T) {
			// EXERCISE
			parsed, err := schedule.Parse(written)

			// VERIFY
			if err != nil {
				t.Fatalf("Parse(%q) returned an unexpected error: %v", written, err)
			}
			if !parsed.RunsAtStartup() {
				t.Errorf("RunsAtStartup() of %q is false, want true", written)
			}
			if parsed.String() != written {
				t.Errorf("String() = %q, want %q: the descriptor is kept as it was written",
					parsed.String(), written)
			}
		})
	}
}

func TestOnlyRebootRunsWhenTheSchedulerStarts(t *testing.T) {
	for _, expression := range clockExpressions {
		t.Run(expression, func(t *testing.T) {
			// EXERCISE
			parsed := mustParse(t, expression)

			// VERIFY
			if parsed.RunsAtStartup() {
				t.Errorf("RunsAtStartup() of %q is true, want false: it is triggered by the clock", expression)
			}
		})
	}
}

func TestARebootScheduleHasNoActivationOnTheClock(t *testing.T) {
	// SETUP
	parsed := mustParse(t, "@reboot")

	for _, instant := range descriptorInstants {
		// EXERCISE
		next, ok := parsed.Next(mustTime(t, instant))

		// VERIFY
		if ok {
			t.Errorf("Next(%s) = %s, want no activation: a reboot job runs when the scheduler starts",
				instant, next)
		}
	}
}

// An unknown descriptor is answered with every one cronx has, the one that runs
// when the scheduler starts included.
func TestAnUnknownDescriptorIsAnsweredWithRebootToo(t *testing.T) {
	// SETUP
	const expression = "@reboots"

	// EXERCISE
	_, err := schedule.Parse(expression)

	// VERIFY
	if err == nil {
		t.Fatalf("Parse(%q) succeeded, want an error", expression)
	}
	if !strings.Contains(err.Error(), "@reboot") {
		t.Errorf("Parse(%q) error = %q, want it to name %q", expression, err, "@reboot")
	}
}
