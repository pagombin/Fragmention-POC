// Command mfpoc is the binary entrypoint. It exposes a cobra CLI with
// `server`, `version`, `config-validate`, and `purge` subcommands. See
// docs/OPERATIONS.md for operator-facing workflows.
package main

import (
	"fmt"
	"os"

	"github.com/pagombin/fragmention-poc/cmd/mfpoc/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
