// Package cli contains the cobra command tree for the mfpoc binary.
// Subcommands live in their own files for clarity: server.go, version.go,
// configvalidate.go, purge.go.
package cli

import (
	"github.com/spf13/cobra"
)

var configPath string

// Root returns the assembled command tree. Called from main and from tests.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "mfpoc",
		Short:         "MongoDB fragmentation reclamation POC platform",
		Long:          "mfpoc loads, fragments, measures, and reclaims MongoDB storage to compare compact vs. initial-sync strategies.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config.yaml")

	root.AddCommand(newServerCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newConfigValidateCmd())
	root.AddCommand(newPurgeCmd())
	return root
}
