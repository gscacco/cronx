package integration_test

import (
	"strings"
	"testing"
	"time"
)

// The zone the fixture configures, and its offset. Fourteen hours ahead of UTC
// puts the clock of the scheduler fourteen hours away from the clock of the
// machine: the tests start cronx with TZ=UTC, so the wall clock it prints for a
// job can only come from the configured zone.
const (
	fixtureZone       = "Pacific/Kiritimati"
	fixtureZoneOffset = 14 * 60 * 60
)

func TestListComputesTheNextRunInTheConfiguredTimezone(t *testing.T) {
	// SETUP
	environment := newEnvironment(t)
	configPath := environment.configure("timezone.toml")

	// EXERCISE
	listed := environment.runOK("list", "--config", configPath)

	// VERIFY: the activation is the next five-minute boundary on the clock of
	// the configured zone, and not on the clock of the machine.
	wall, printed := nextActivation(t, listed.stdout, "offset")
	got, err := time.ParseInLocation("2006-01-02 15:04:05", wall, time.FixedZone("+14", fixtureZoneOffset))
	if err != nil {
		t.Fatalf("reading the activation list printed (%q): %v", wall, err)
	}
	if elapsed := got.Sub(time.Now()); elapsed < -5*time.Minute || elapsed > 5*time.Minute {
		t.Errorf("list printed the next run as %q (%s), want a wall clock within five minutes of now in %s",
			wall, printed, fixtureZone)
	}
}

// nextActivation returns the wall clock "cronx list" printed in the NEXT column
// of a job, and the zone it printed it in.
func nextActivation(t *testing.T, listing, job string) (wall, zone string) {
	t.Helper()
	for _, line := range strings.Split(listing, "\n") {
		// The columns are padded, so a cell is separated from its neighbour by
		// two spaces and a schedule keeps the single spaces of a cron
		// expression.
		var cells []string
		for _, cell := range strings.Split(line, "  ") {
			if trimmed := strings.TrimSpace(cell); trimmed != "" {
				cells = append(cells, trimmed)
			}
		}
		if len(cells) < 3 || cells[0] != job {
			continue
		}
		fields := strings.Fields(cells[2])
		if len(fields) != 3 {
			t.Fatalf("the NEXT column of %q is %q, want a date, a time and a zone", job, cells[2])
		}
		return fields[0] + " " + fields[1], fields[2]
	}
	t.Fatalf("list printed no line for the job %q:\n%s", job, listing)
	return "", ""
}
