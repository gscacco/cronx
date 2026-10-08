package config_test

import (
	"strings"
	"testing"
	"time"

	"gscacco.com/cronx/internal/config"
)

func TestSchedulerLocationResolvesTheZoneOfTheMachine(t *testing.T) {
	// SETUP: the two names a configuration can use for "the zone of the machine".
	// A machine may have no zone of its own, in which case that zone is UTC:
	// which is why the result is compared with the location itself and not with
	// a name.
	subjects := map[string]string{
		"the default":   config.DefaultTimezone,
		"an empty name": "",
	}

	for description, timezone := range subjects {
		t.Run(description, func(t *testing.T) {
			// EXERCISE
			location, err := config.Scheduler{Timezone: timezone}.Location()

			// VERIFY
			if err != nil {
				t.Fatalf("Location() returned an unexpected error: %v", err)
			}
			if location != time.Local {
				t.Errorf("Location() = %s, want the zone of the machine", location)
			}
		})
	}
}

func TestSchedulerLocationResolvesAnIANAZoneName(t *testing.T) {
	// SETUP
	subject := config.Scheduler{Timezone: "Europe/Rome"}

	// EXERCISE
	location, err := subject.Location()

	// VERIFY
	if err != nil {
		t.Fatalf("Location() returned an unexpected error: %v", err)
	}
	if got, want := location.String(), "Europe/Rome"; got != want {
		t.Errorf("Location() = %q, want %q", got, want)
	}
}

func TestSchedulerLocationRejectsAnUnknownTimezone(t *testing.T) {
	// SETUP
	subject := config.Scheduler{Timezone: "Europe/Roma"}

	// EXERCISE
	location, err := subject.Location()

	// VERIFY
	if err == nil {
		t.Fatalf("Location() = %s, want an error for a name no zone has", location)
	}
	if !strings.Contains(err.Error(), `scheduler.timezone "Europe/Roma" is not a known timezone`) {
		t.Errorf("Location() error = %q, want it to name the timezone it cannot resolve", err)
	}
}

func TestParseRejectsAnUnknownTimezone(t *testing.T) {
	// SETUP
	data := []byte("[scheduler]\ntimezone = \"Europe/Roma\"\n")

	// EXERCISE
	_, err := config.Parse(data)

	// VERIFY
	if err == nil {
		t.Fatal("Parse() succeeded, want an unknown timezone to be rejected")
	}
	if !strings.Contains(err.Error(), `scheduler.timezone "Europe/Roma" is not a known timezone`) {
		t.Errorf("Parse() error = %q, want it to report the timezone", err)
	}
}
