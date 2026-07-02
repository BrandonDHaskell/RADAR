// Command radar is RADAR's single binary: it runs one-shot CLI commands and,
// via `radar serve`, a long-lived background service. Both modes call the
// same internal/ packages; the CLI itself holds no domain logic.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
