package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pagombin/fragmention-poc/internal/config"
)

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config-validate",
		Short: "Validate the supplied config file without starting the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "config OK")
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "mongo.uri (redacted):", config.RedactMongoURI(cfg.Mongo.ResolvedURI))
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "mongo.is_srv:", cfg.Mongo.IsSRV)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "server.listen:", cfg.Server.Listen)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "auth.enabled:", cfg.Auth.Enabled)
			return nil
		},
	}
}
