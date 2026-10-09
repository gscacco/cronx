package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// sizeUnits are the suffixes a configured size may carry, largest first, so
// that "MB" is not read as "B".
var sizeUnits = []struct {
	suffix string
	factor int64
}{
	{"GB", 1 << 30},
	{"MB", 1 << 20},
	{"KB", 1 << 10},
	{"B", 1},
}

// parseSize reads a size written as a number of bytes, or as a number followed
// by KB, MB or GB. The suffix is case-insensitive. It fails on anything else,
// and on a size that is not greater than zero.
func parseSize(value string) (int64, error) {
	number := strings.ToUpper(strings.TrimSpace(value))

	factor := int64(1)
	for _, unit := range sizeUnits {
		if strings.HasSuffix(number, unit.suffix) {
			factor = unit.factor
			number = strings.TrimSuffix(number, unit.suffix)
			break
		}
	}

	amount, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number of bytes, optionally with a KB, MB or GB suffix", value)
	}
	if amount <= 0 {
		return 0, fmt.Errorf("%q is not greater than zero", value)
	}
	if amount > math.MaxInt64/factor {
		return 0, fmt.Errorf("%q is larger than the largest size cronx can hold", value)
	}
	return amount * factor, nil
}
