package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/pagombin/fragmention-poc/internal/api"
	"github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/logging"
	"github.com/pagombin/fragmention-poc/internal/storage"
	"github.com/pagombin/fragmention-poc/internal/version"
)

func newServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Run the mfpoc HTTP server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			logger := logging.Init(logging.Config{
				Level:  cfg.Logging.Level,
				Format: logging.Format(cfg.Logging.Format),
			})

			info := version.Get()
			logger.Info().
				Str("version", info.Version).
				Str("commit", info.Commit).
				Str("build_time", info.BuildTime).
				Str("mongo_uri", config.RedactMongoURI(cfg.Mongo.ResolvedURI)).
				Bool("mongo_is_srv", cfg.Mongo.IsSRV).
				Str("listen", cfg.Server.Listen).
				Bool("tls", cfg.Server.TLS.Enabled).
				Msg("mfpoc starting")

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			store, err := storage.Open(ctx, storage.Config{
				Path:        cfg.Storage.Path,
				BusyTimeout: cfg.Storage.BusyTimeout,
			})
			if err != nil {
				return fmt.Errorf("open storage: %w", err)
			}
			defer func() { _ = store.Close() }()

			deps := api.Deps{
				Cfg:    cfg,
				Logger: logger,
				Store:  store,
				Readyz: func(ctx context.Context) error { return store.Ping(ctx) },
			}
			handler := api.NewRouter(deps)
			srv, err := api.NewServer(cfg, logger, handler)
			if err != nil {
				return fmt.Errorf("new server: %w", err)
			}
			return srv.Start(ctx)
		},
	}
}
