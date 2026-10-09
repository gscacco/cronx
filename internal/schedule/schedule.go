// Package schedule parses cron expressions and computes their next activation.
//
// The supported syntax is deliberately close to traditional Unix cron: five
// fields (minute, hour, day of month, month, day of week), the operators "*",
// ranges "a-b", lists "a,b" and the steps "*/n" and "a-b/n", the three-letter
// month and day names, and the descriptors "@yearly", "@monthly", "@weekly",
// "@daily", "@midnight" and "@hourly", which stand for a fixed time, together
// with "@reboot", which runs the job when the scheduler starts. A seconds field
// and the non-standard "L", "W" and "#" operators are not supported.
//
// When both the day-of-month and the day-of-week fields are restricted, a day
// matches if either of them matches, following the traditional cron behaviour.
package schedule

import (
	"fmt"
	"strings"
	"time"
)

// fieldCount is the number of fields in a cron expression.
const fieldCount = 5

// maxSearchYears bounds the search performed by Next. The largest possible gap
// between two occurrences is eight years (29 February around a century that is
// not a leap year, for example 2096 and 2104), so twelve years is enough.
const maxSearchYears = 12

// Schedule is a parsed cron expression.
//
// The zero value is not usable: obtain a Schedule with Parse. A Schedule is
// immutable and therefore safe for concurrent use.
type Schedule struct {
	expression  string
	minutes     uint64
	hours       uint64
	daysOfMonth uint64
	months      uint64
	daysOfWeek  uint64
	// domStar and dowStar record whether the field was written as "*".
	domStar bool
	dowStar bool
	// startup records that the schedule is triggered by the scheduler
	// starting rather than by the clock: it is "@reboot", which has no
	// activation to compute.
	startup bool
}

// String returns the expression the Schedule was parsed from.
func (s Schedule) String() string {
	return s.expression
}

// RunsAtStartup reports whether the schedule is triggered by the scheduler
// starting rather than by the clock, which is what the "@reboot" descriptor
// means and the only way to ask for it.
func (s Schedule) RunsAtStartup() bool {
	return s.startup
}

// Parse parses a cron expression: five fields, or one of the descriptors that
// stands for a fixed time, such as "@daily".
func Parse(expression string) (Schedule, error) {
	if isDescriptor(expression) {
		return parseDescriptor(expression)
	}
	return parseFields(expression, expression)
}

// parseFields parses the five fields of expression. written is the text the
// caller passed, which the Schedule reports back even when the fields come from
// a descriptor.
func parseFields(written, expression string) (Schedule, error) {
	fields := strings.Fields(expression)
	if len(fields) != fieldCount {
		return Schedule{}, fmt.Errorf(
			"cron expression %q must have %d fields, got %d", expression, fieldCount, len(fields))
	}

	parsed := Schedule{expression: written}
	var err error

	if parsed.minutes, _, err = parseField(minuteField, fields[0]); err != nil {
		return Schedule{}, err
	}
	if parsed.hours, _, err = parseField(hourField, fields[1]); err != nil {
		return Schedule{}, err
	}
	if parsed.daysOfMonth, parsed.domStar, err = parseField(dayOfMonthField, fields[2]); err != nil {
		return Schedule{}, err
	}
	if parsed.months, _, err = parseField(monthField, fields[3]); err != nil {
		return Schedule{}, err
	}
	if parsed.daysOfWeek, parsed.dowStar, err = parseField(dayOfWeekField, fields[4]); err != nil {
		return Schedule{}, err
	}

	return parsed, nil
}

// Next returns the first activation strictly after the given time, expressed in
// the location of that time. The boolean is false when the expression has no
// activation within the search horizon, for example "0 0 31 4 *", which can
// never match because April has no 31st day, and for a schedule that runs when
// the scheduler starts rather than on the clock, which has no activation to
// compute at all.
func (s Schedule) Next(after time.Time) (time.Time, bool) {
	if s.startup {
		return time.Time{}, false
	}

	// The first candidate is the start of the following minute.
	candidate := time.Date(
		after.Year(), after.Month(), after.Day(),
		after.Hour(), after.Minute(), 0, 0, after.Location(),
	).Add(time.Minute)

	horizon := candidate.AddDate(maxSearchYears, 0, 0)

	for !candidate.After(horizon) {
		switch {
		case !hasBit(s.months, int(candidate.Month())):
			candidate = startOfNextMonth(candidate)
		case !s.matchesDay(candidate):
			candidate = startOfNextDay(candidate)
		case !hasBit(s.hours, candidate.Hour()):
			candidate = startOfNextHour(candidate)
		case !hasBit(s.minutes, candidate.Minute()):
			candidate = candidate.Add(time.Minute)
		default:
			return candidate, true
		}
	}

	return time.Time{}, false
}

// matchesDay reports whether the calendar day matches the day-of-month and
// day-of-week fields. When both are restricted, either matching is enough.
func (s Schedule) matchesDay(t time.Time) bool {
	dayOfMonth := hasBit(s.daysOfMonth, t.Day())
	dayOfWeek := hasBit(s.daysOfWeek, int(t.Weekday()))

	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dayOfWeek
	case s.dowStar:
		return dayOfMonth
	default:
		return dayOfMonth || dayOfWeek
	}
}

// hasBit reports whether the given bit is set in mask.
func hasBit(mask uint64, bit int) bool {
	return mask&(1<<uint(bit)) != 0
}

// startOfNextHour returns the following hour boundary.
func startOfNextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(time.Hour)
}

// startOfNextDay returns midnight at the start of the following day.
func startOfNextDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

// startOfNextMonth returns midnight at the start of the following month.
func startOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
}
