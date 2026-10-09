package schedule

import (
	"fmt"
	"strings"
)

// descriptors are the shorthands cronx accepts for a fixed time, each with the
// five-field expression it stands for, in the order docs/scheduling.md lists
// them. Traditional cron defines the same six.
var descriptors = []struct {
	name       string
	expression string
}{
	{name: "@yearly", expression: "0 0 1 1 *"},
	{name: "@monthly", expression: "0 0 1 * *"},
	{name: "@weekly", expression: "0 0 * * 0"},
	{name: "@daily", expression: "0 0 * * *"},
	{name: "@midnight", expression: "0 0 * * *"},
	{name: "@hourly", expression: "0 * * * *"},
}

// isDescriptor reports whether an expression is written as a descriptor, which
// is what the "@" marker means: a descriptor is a whole expression, never a
// field of one.
func isDescriptor(expression string) bool {
	return strings.HasPrefix(strings.TrimSpace(expression), "@")
}

// parseDescriptor parses a descriptor by expanding it into the expression it
// stands for. The descriptor is kept as the expression of the Schedule, so that
// a job is reported as its author wrote it.
func parseDescriptor(expression string) (Schedule, error) {
	fields := strings.Fields(expression)
	if len(fields) != 1 {
		return Schedule{}, fmt.Errorf(
			"cron expression %q is not valid: a descriptor such as @daily stands alone, with no fields",
			expression)
	}

	for _, descriptor := range descriptors {
		if strings.EqualFold(fields[0], descriptor.name) {
			return parseFields(expression, descriptor.expression)
		}
	}

	return Schedule{}, fmt.Errorf("cron expression %q is not a known descriptor: use one of %s",
		expression, descriptorList())
}

// descriptorList returns the descriptors cronx accepts, as an error message
// lists them.
func descriptorList() string {
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		names = append(names, descriptor.name)
	}
	return strings.Join(names, ", ")
}
