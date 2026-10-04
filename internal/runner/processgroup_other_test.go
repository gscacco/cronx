//go:build !unix

package runner_test

import "errors"

// recordProcessGroup is not available on platforms without Unix process groups.
func recordProcessGroup() (string, error) {
	return "", errors.New("process groups are not available on this platform")
}
