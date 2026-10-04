// Command cronx is the entry point of the cronx job scheduler.
package main

import (
	"fmt"
	"os"

	"gscacco.com/cronx/internal/cli"
)

// exitFailure is the exit status used when the command fails.
const exitFailure = 1

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "cronx:", err)
		os.Exit(exitFailure)
	}
}
