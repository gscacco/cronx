package schedule

import (
	"fmt"
	"strconv"
	"strings"
)

// fieldSpec describes one field of a cron expression.
type fieldSpec struct {
	// name is used in error messages.
	name string
	// min and max bound the accepted values, inclusive.
	min, max int
	// names maps symbolic names (case-insensitive) to their numbers.
	names map[string]int
	// normalize rewrites an accepted number into its canonical form.
	normalize func(int) int
	// operators records that the field is one of the two day fields, which
	// alone accept the "L", "W" and "#" operators. The others name the day
	// fields when one of the operators turns up in them, instead of
	// reporting it as a value they do not know.
	operators bool
}

var (
	// secondField is the optional field: a six-field expression carries it in
	// front of the five the others make up.
	secondField = fieldSpec{name: "second", min: 0, max: 59}
	minuteField = fieldSpec{name: "minute", min: 0, max: 59}
	hourField   = fieldSpec{name: "hour", min: 0, max: 23}

	// The two day fields are the ones that accept the day operators; see
	// dayops.go.
	dayOfMonthField = fieldSpec{name: "day of month", min: 1, max: 31, operators: true}
	monthField      = fieldSpec{name: "month", min: 1, max: 12, names: monthNames}

	// Sunday may be written as 0 or as 7, so the day-of-week field accepts
	// values up to 7 and normalizes 7 to 0.
	dayOfWeekField = fieldSpec{
		name:      "day of week",
		min:       0,
		max:       7,
		names:     dayNames,
		normalize: normalizeWeekday,
		operators: true,
	}
)

// monthNames maps the accepted month names to their numbers.
var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// dayNames maps the accepted day names to their numbers (Sunday is 0).
var dayNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

// normalizeWeekday maps Sunday written as 7 to 0, as traditional cron does.
func normalizeWeekday(value int) int {
	if value == 7 {
		return 0
	}
	return value
}

// parseField parses one field into a bitmask. The boolean reports whether the
// field was written as the literal "*", which distinguishes "every value" from
// "a restriction that happens to cover every value".
func parseField(field fieldSpec, expression string) (uint64, bool, error) {
	if expression == "" {
		return 0, false, fmt.Errorf("%s field is missing", field.name)
	}

	var mask uint64
	for _, item := range strings.Split(expression, ",") {
		itemMask, err := parseItem(field, item)
		if err != nil {
			return 0, false, err
		}
		mask |= itemMask
	}
	return mask, expression == "*", nil
}

// parseItem parses one comma-separated element of a field: a value, a range, a
// star, or one of those followed by a step ("*/n", "a-b/n").
func parseItem(field fieldSpec, item string) (uint64, error) {
	if item == "" {
		return 0, fmt.Errorf("%s field has an empty list item", field.name)
	}

	base := item
	step := 1
	if slash := strings.IndexByte(item, '/'); slash >= 0 {
		base = item[:slash]
		text := item[slash+1:]

		parsed, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("%s field: invalid step in %q: %q is not a number", field.name, item, text)
		}
		if parsed < 1 {
			return 0, fmt.Errorf("%s field: invalid step in %q: it must be at least 1", field.name, item)
		}
		if base != "*" && !strings.Contains(base, "-") {
			return 0, fmt.Errorf("%s field: %q is not valid: use %q or a range such as %q",
				field.name, item, "*/"+text, "a-b/"+text)
		}
		step = parsed
	}

	low, high, err := parseRange(field, base)
	if err != nil {
		return 0, err
	}

	var mask uint64
	for value := low; value <= high; value += step {
		mask |= bitFor(field, value)
	}
	return mask, nil
}

// parseRange parses a star, a single value or an inclusive range "a-b".
func parseRange(field fieldSpec, base string) (int, int, error) {
	if base == "*" {
		return field.min, field.max, nil
	}

	if dash := strings.IndexByte(base, '-'); dash >= 0 {
		low, err := parseValue(field, base[:dash])
		if err != nil {
			return 0, 0, err
		}
		high, err := parseValue(field, base[dash+1:])
		if err != nil {
			return 0, 0, err
		}
		if low > high {
			return 0, 0, fmt.Errorf("%s field: range %q is reversed", field.name, base)
		}
		return low, high, nil
	}

	value, err := parseValue(field, base)
	if err != nil {
		return 0, 0, err
	}
	return value, value, nil
}

// parseValue parses a single value, accepting a symbolic name where the field
// defines one, and checks that it is within the field's range.
func parseValue(field fieldSpec, text string) (int, error) {
	if text == "" {
		return 0, fmt.Errorf("%s field has an empty value", field.name)
	}
	if value, ok := field.names[strings.ToLower(text)]; ok {
		return value, nil
	}

	value, err := strconv.Atoi(text)
	if err != nil {
		if !field.operators && strings.ContainsAny(text, "LW#") {
			return 0, fmt.Errorf(
				"%s field: %q is not a valid value: the L, W and # day operators belong to the day of month and day of week fields",
				field.name, text)
		}
		return 0, fmt.Errorf("%s field: %q is not a valid value", field.name, text)
	}
	if value < field.min || value > field.max {
		return 0, fmt.Errorf("%s field: %d is out of range [%d-%d]", field.name, value, field.min, field.max)
	}
	return value, nil
}

// bitFor returns the mask bit for an accepted value.
func bitFor(field fieldSpec, value int) uint64 {
	if field.normalize != nil {
		value = field.normalize(value)
	}
	return 1 << uint(value)
}
