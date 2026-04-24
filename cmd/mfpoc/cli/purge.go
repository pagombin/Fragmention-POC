package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// newPurgeCmd is a placeholder for Phase-21 "Purge POC data" CLI support.
// The actual purge logic lives alongside the HTTP handler so the UI and CLI
// share one implementation.
func newPurgeCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "purge",
		Short:  "Drop all databases matching the configured POC prefix (requires typed confirmation)",
		RunE:   func(cmd *cobra.Command, args []string) error { return errors.New("purge is not yet implemented; use the UI or wait for Phase 17+") },
	}
}
