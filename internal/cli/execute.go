package cli

import (
	"fmt"
	"os"
)

// Execute is the CLI entrypoint.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		// Cobra already prints command-specific errors in many cases; keep this concise.
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
