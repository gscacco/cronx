package config

import (
	"fmt"
	"time"

	// The zone database travels with the binary. A job is only where the
	// configuration says it is when the zone it names resolves, and it has to
	// resolve on the machines cronx is installed on: a minimal container, a Nix
	// store, any host without /usr/share/zoneinfo.
	_ "time/tzdata"
)

// Location resolves the configured timezone to the location the scheduler
// computes activation times in. An empty name and "Local" mean the zone of the
// machine; any other value must be a name the zone database knows.
func (s Scheduler) Location() (*time.Location, error) {
	name := s.Timezone
	if name == "" {
		name = DefaultTimezone
	}

	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("scheduler.timezone %q is not a known timezone", s.Timezone)
	}
	return location, nil
}
