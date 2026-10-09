package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// This file holds the two day fields, which are the only ones that accept the
// "L", "W" and "#" operators. Those name days that a plain value cannot: the
// last day of a month, the weekday nearest to one, the last occurrence of a
// weekday in a month and the n-th occurrence of one.
//
// Every operator is a whole element of the field's list, so none of them carries
// a range or a step: "1,15,L" is the first, the fifteenth and the last day of
// the month, while "L/2" and "1-L" are refused rather than guessed at.

// dayOfMonth is the parsed day-of-month field: the days its plain values select,
// together with the days its operators select.
type dayOfMonth struct {
	// days holds the days named as plain values.
	days uint64
	// star records that the field was written as the literal "*", which is
	// what tells "every day" from "a restriction that happens to cover every
	// day".
	star bool
	// last records the "L" operator: the last day of the month.
	last bool
	// lastWeekday records the "LW" operator: the last weekday of the month.
	lastWeekday bool
	// nearestWeekdays holds the days named as "<n>W": the weekday nearest to
	// each of them, a day that only some months hold.
	nearestWeekdays []int
}

// dayOfWeek is the parsed day-of-week field: the weekdays its plain values
// select, together with the weekdays its operators select.
type dayOfWeek struct {
	// days holds the weekdays named as plain values, Sunday being 0.
	days uint64
	// star records that the field was written as the literal "*".
	star bool
	// lastInMonth holds the weekdays named as "<n>L": the last occurrence of
	// each of them in the month.
	lastInMonth uint64
	// nth holds the weekdays named as "<n>#<m>": the m-th occurrence of each
	// of them in the month.
	nth []nthWeekday
}

// nthWeekday is one "<weekday>#<n>" element: the n-th occurrence of a weekday
// in the month.
type nthWeekday struct {
	weekday int // 0 is Sunday
	n       int // 1 to 5
}

// matches reports whether the calendar day of t is selected by the field.
func (d dayOfMonth) matches(t time.Time) bool {
	if hasBit(d.days, t.Day()) {
		return true
	}
	if d.last && t.Day() == lastDayOfMonth(t) {
		return true
	}
	if d.lastWeekday && t.Day() == lastWeekdayOfMonth(t) {
		return true
	}
	for _, day := range d.nearestWeekdays {
		if t.Day() == nearestWeekdayOfMonth(t, day) {
			return true
		}
	}
	return false
}

// matches reports whether the calendar day of t is selected by the field.
func (d dayOfWeek) matches(t time.Time) bool {
	weekday := int(t.Weekday())
	if hasBit(d.days, weekday) {
		return true
	}
	// The last occurrence of a weekday in a month is the one that no later
	// day of that month shares its weekday with.
	if hasBit(d.lastInMonth, weekday) && t.Day()+7 > lastDayOfMonth(t) {
		return true
	}
	for _, nth := range d.nth {
		if weekday == nth.weekday && occurrenceInMonth(t) == nth.n {
			return true
		}
	}
	return false
}

// lastDayOfMonth returns the day of the month that ends the month of t.
func lastDayOfMonth(t time.Time) int {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return first.AddDate(0, 1, -1).Day()
}

// occurrenceInMonth returns which occurrence of its weekday a day is: 1 for the
// first Monday of a month, 2 for the second, and so on.
func occurrenceInMonth(t time.Time) int {
	return (t.Day()-1)/7 + 1
}

// lastWeekdayOfMonth returns the day the "LW" operator selects in the month of
// t: the last day of it that is neither a Saturday nor a Sunday.
func lastWeekdayOfMonth(t time.Time) int {
	day := time.Date(t.Year(), t.Month(), lastDayOfMonth(t), 0, 0, 0, 0, t.Location())
	switch day.Weekday() {
	case time.Saturday:
		return day.Day() - 1
	case time.Sunday:
		return day.Day() - 2
	default:
		return day.Day()
	}
}

// nearestWeekdayOfMonth returns the day the "<n>W" operator selects in the month
// of t: the weekday nearest to day n, before or after it, and never outside the
// month. A day that is itself a weekday is its own answer, a Saturday gives the
// Friday before it unless that would leave the month, and a Sunday gives the
// Monday after it unless that would leave the month.
//
// It returns zero for a month that does not hold day n, which no day of a month
// is, so that the operator selects no day there rather than a day of a
// neighbouring month.
func nearestWeekdayOfMonth(t time.Time, n int) int {
	if n > lastDayOfMonth(t) {
		return 0
	}
	day := time.Date(t.Year(), t.Month(), n, 0, 0, 0, 0, t.Location())
	switch day.Weekday() {
	case time.Saturday:
		if day.Day() == 1 {
			return n + 2
		}
		return n - 1
	case time.Sunday:
		if day.Day() == lastDayOfMonth(t) {
			return n - 2
		}
		return n + 1
	default:
		return n
	}
}

// parseDayOfMonth parses the day-of-month field, one of the two that accept the
// day operators.
func parseDayOfMonth(expression string) (dayOfMonth, error) {
	if expression == "" {
		return dayOfMonth{}, fmt.Errorf("%s field is missing", dayOfMonthField.name)
	}

	parsed := dayOfMonth{star: expression == "*"}
	for _, item := range strings.Split(expression, ",") {
		switch {
		case item == "L":
			parsed.last = true
		case item == "LW":
			parsed.lastWeekday = true
		case isNearestWeekday(item):
			day, err := parseValue(dayOfMonthField, strings.TrimSuffix(item, "W"))
			if err != nil {
				return dayOfMonth{}, err
			}
			parsed.nearestWeekdays = append(parsed.nearestWeekdays, day)
		case strings.Contains(item, "#"):
			return dayOfMonth{}, fmt.Errorf(
				"%s field: %q is not valid: the %q operator belongs to the day of week field",
				dayOfMonthField.name, item, "#")
		case strings.ContainsAny(item, "LW"):
			return dayOfMonth{}, fmt.Errorf(
				"%s field: %q is not valid: use %q for the last day of the month, %q for the last weekday of it, or %q for the weekday nearest to a day",
				dayOfMonthField.name, item, "L", "LW", "<day>W")
		default:
			itemMask, err := parseItem(dayOfMonthField, item)
			if err != nil {
				return dayOfMonth{}, err
			}
			parsed.days |= itemMask
		}
	}
	return parsed, nil
}

// parseDayOfWeek parses the day-of-week field, the other one that accepts the
// day operators.
func parseDayOfWeek(expression string) (dayOfWeek, error) {
	if expression == "" {
		return dayOfWeek{}, fmt.Errorf("%s field is missing", dayOfWeekField.name)
	}

	parsed := dayOfWeek{star: expression == "*"}
	for _, item := range strings.Split(expression, ",") {
		switch {
		case item == "L":
			// The last day of the week, which is Saturday where Sunday is
			// the first day of it.
			parsed.days |= bitFor(dayOfWeekField, int(time.Saturday))
		case item == "W" || item == "LW" || isNearestWeekday(item):
			return dayOfWeek{}, fmt.Errorf(
				"%s field: %q is not valid: the %q operator belongs to the day of month field",
				dayOfWeekField.name, item, "W")
		case isLastInMonth(item):
			weekday, err := parseValue(dayOfWeekField, strings.TrimSuffix(item, "L"))
			if err != nil {
				return dayOfWeek{}, err
			}
			parsed.lastInMonth |= bitFor(dayOfWeekField, weekday)
		case strings.Contains(item, "#"):
			weekday, nth, err := parseNthWeekday(item)
			if err != nil {
				return dayOfWeek{}, err
			}
			parsed.nth = append(parsed.nth, nthWeekday{weekday: weekday, n: nth})
		case strings.Contains(item, "L"):
			return dayOfWeek{}, fmt.Errorf(
				"%s field: %q is not valid: use %q for the last day of the week, %q for the last occurrence of a weekday in the month, or %q for the n-th one",
				dayOfWeekField.name, item, "L", "<day>L", "<day>#<n>")
		default:
			itemMask, err := parseItem(dayOfWeekField, item)
			if err != nil {
				return dayOfWeek{}, err
			}
			parsed.days |= itemMask
		}
	}
	return parsed, nil
}

// isNearestWeekday reports whether an element is written as "<n>W", the operator
// that names the weekday nearest to a day of the month.
func isNearestWeekday(item string) bool {
	day, ok := strings.CutSuffix(item, "W")
	if !ok {
		return false
	}
	_, err := strconv.Atoi(day)
	return err == nil
}

// isLastInMonth reports whether an element is written as "<weekday>L", the
// operator that names the last occurrence of a weekday in the month. The bare
// "L" is the last day of the week, and is handled before this.
func isLastInMonth(item string) bool {
	weekday, ok := strings.CutSuffix(item, "L")
	return ok && weekday != ""
}

// parseNthWeekday parses an element written as "<weekday>#<n>", the operator
// that names the n-th occurrence of a weekday in the month, and returns the
// weekday as a number from 0 to 6 and the occurrence as a number from 1 to 5.
func parseNthWeekday(item string) (weekday, nth int, err error) {
	weekdayText, nthText, _ := strings.Cut(item, "#")

	weekday, err = parseValue(dayOfWeekField, weekdayText)
	if err != nil {
		return 0, 0, err
	}

	nth, err = strconv.Atoi(nthText)
	if err != nil {
		return 0, 0, fmt.Errorf(
			"%s field: %q is not valid: %q is not the number of an occurrence",
			dayOfWeekField.name, item, nthText)
	}
	if nth < 1 || nth > 5 {
		return 0, 0, fmt.Errorf(
			"%s field: %q is not valid: a month holds between one and five occurrences of a weekday, got %d",
			dayOfWeekField.name, item, nth)
	}
	return normalizeWeekday(weekday), nth, nil
}
