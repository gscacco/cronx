// Package schedule parses cron expressions and computes their next activation.
//
// The supported syntax is deliberately close to traditional Unix cron: five
// fields (minute, hour, day of month, month, day of week) or six, with the
// seconds field in front of them, the operators "*", ranges "a-b", lists "a,b"
// and the steps "*/n" and "a-b/n", the three-letter month and day names, and the
// descriptors "@yearly", "@monthly", "@weekly", "@daily", "@midnight" and
// "@hourly", which stand for a fixed time, together with "@reboot", which runs
// the job when the scheduler starts. The seconds field is the one a five-field
// expression leaves unsaid, and it means zero there: an activation is on the
// minute unless the expression says otherwise. The non-standard "L", "W" and "#"
// operators are not supported.
//
// When both the day-of-month and the day-of-week fields are restricted, a day
// matches if either of them matches, following the traditional cron behaviour.
package schedule

import (
	"fmt"
	"strings"
	"time"
)

// fieldCount is the number of fields in a traditional cron expression.
const fieldCount = 5

// maxFieldCount is the number of fields of an expression that states the
// seconds as well: the seconds field is the only optional one, and it comes
// first.
const maxFieldCount = fieldCount + 1

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
	seconds     uint64
	minutes     uint64
	hours       uint64
	daysOfMonth uint64
	months      uint64
	daysOfWeek  uint64
	// domStar and dowStar record whether the field was written as "*".
	domStar bool
	dowStar bool
	// hasSeconds records that the expression states the seconds as well, which
	// is what a six-field expression does: it is the difference between an
	// activation that may be anywhere inside the minute and one that is always
	// on the minute.
	hasSeconds bool
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

// Parse parses a cron expression: five fields, six with the seconds field in
// front of them, or one of the descriptors that stands for a fixed time, such as
// "@daily".
func Parse(expression string) (Schedule, error) {
	if isDescriptor(expression) {
		return parseDescriptor(expression)
	}
	return parseFields(expression, expression)
}

// parseFields parses the fields of expression: the five a traditional cron
// expression has, and the optional seconds field in front of them. written is
// the text the caller passed, which the Schedule reports back even when the
// fields come from a descriptor.
func parseFields(written, expression string) (Schedule, error) {
	fields := strings.Fields(expression)
	if len(fields) != fieldCount && len(fields) != maxFieldCount {
		return Schedule{}, fmt.Errorf(
			"cron expression %q must have %d fields, or %d with the seconds field first, got %d",
			expression, fieldCount, maxFieldCount, len(fields))
	}

	parsed := Schedule{expression: written}
	var err error

	if len(fields) == maxFieldCount {
		// The seconds field is the first one, and the only optional one.
		parsed.hasSeconds = true
		if parsed.seconds, _, err = parseField(secondField, fields[0]); err != nil {
			return Schedule{}, err
		}
		fields = fields[1:]
	} else {
		// Without a seconds field an activation is on the minute, which is
		// what the expressions written before it meant.
		parsed.seconds = bitFor(secondField, 0)
	}

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
//
// An activation of an expression that states no seconds is on the minute, as it
// always was; a six-field expression is asked for activations inside the minute
// as well.
func (s Schedule) Next(after time.Time) (time.Time, bool) {
	if s.startup {
		return time.Time{}, false
	}

	candidate := s.firstCandidate(after)
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
			candidate = startOfNextMinute(candidate)
		case !hasBit(s.seconds, candidate.Second()):
			candidate = candidate.Add(time.Second)
		default:
			return candidate, true
		}
	}

	return time.Time{}, false
}

// firstCandidate returns the earliest instant that could be an activation after
// the given one: the following second for an expression that states the seconds,
// and the start of the following minute for one that does not, which is the
// earliest instant such an expression can match at all.
func (s Schedule) firstCandidate(after time.Time) time.Time {
	if s.hasSeconds {
		return time.Date(
			after.Year(), after.Month(), after.Day(),
			after.Hour(), after.Minute(), after.Second(), 0, after.Location(),
		).Add(time.Second)
	}
	return time.Date(
		after.Year(), after.Month(), after.Day(),
		after.Hour(), after.Minute(), 0, 0, after.Location(),
	).Add(time.Minute)
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

// startOfNextMinute returns the start of the following minute, with the seconds
// back at zero: a step to another minute makes the seconds field apply to that
// minute from its beginning, which is what the field means.
//
// It is worked out from the instant rather than from the wall clock it is in,
// because a minute that daylight saving makes happen twice is a wall clock that
// names two instants: building it back from its parts would pick one of the two
// and land an hour away from the candidate being advanced.
func startOfNextMinute(t time.Time) time.Time {
	return t.Truncate(time.Minute).Add(time.Minute)
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
