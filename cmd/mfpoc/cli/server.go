package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/pagombin/fragmention-poc/internal/api"
	"github.com/pagombin/fragmention-poc/internal/collector"
	"github.com/pagombin/fragmention-poc/internal/config"
	"github.com/pagombin/fragmention-poc/internal/logging"
	"github.com/pagombin/fragmention-poc/internal/metrics"
	mongoClient "github.com/pagombin/fragmention-poc/internal/mongo"
	"github.com/pagombin/fragmention-poc/internal/storage"
	"github.com/pagombin/fragmention-poc/internal/supervisor"
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
			metrics.InfoGauge.WithLabelValues(info.Version, info.Commit, info.GoVersion).Set(1)

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

			mc, err := mongoClient.Connect(ctx, cfg.Mongo)
			if err != nil {
				// Fall back to starting the server even when Mongo is unreachable,
				// so operators can at least hit /health and /api/v1/version on a
				// fresh droplet. /ready will continue to fail until Mongo is up.
				logger.Error().Err(err).Msg("mongo connect failed; continuing in degraded mode")
			} else {
				defer func() {
					shutCtx, cancel := context.WithTimeout(context.Background(), 5*cfg.Mongo.OperationTimeout)
					defer cancel()
					_ = mc.Close(shutCtx)
				}()
				si := mc.ServerInfo()
				metrics.ServerInfo.WithLabelValues(si.Version, si.StorageEngine, si.WiredTigerCodec, boolStr(cfg.Mongo.IsSRV)).Set(1)
				logger.Info().
					Str("server_version", si.Version).
					Str("fcv", si.FCV).
					Msg("mongo connected")
			}

			sup := supervisor.New(logger)
			var col *collector.Collector
			if mc != nil {
				col = collector.New(collector.Config{
					IdleInterval:   cfg.Collector.IdleInterval,
					ActiveInterval: cfg.Collector.ActiveInterval,
					BackoffInitial: cfg.Collector.BackoffInitial,
					BackoffMax:     cfg.Collector.BackoffMax,
				}, mc, storage.NewSamples(store), storage.NewSnapshots(store), storage.NewEvents(store), logger)
				sup.Register(col)
			}

			deps := api.Deps{
				Cfg:       cfg,
				Logger:    logger,
				Store:     store,
				Mongo:     mc,
				Collector: col,
				Readyz: func(ctx context.Context) error {
					if err := store.Ping(ctx); err != nil {
						return fmt.Errorf("storage: %w", err)
					}
					if mc != nil {
						return mc.Ping(ctx)
					}
					return errors.New("mongo not connected")
				},
			}
			handler := api.NewRouter(deps)
			srv, err := api.NewServer(cfg, logger, handler)
			if err != nil {
				return fmt.Errorf("new server: %w", err)
			}

			// Run supervisor + HTTP server concurrently; cancellation of ctx
			// causes both to drain and exit.
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error { return srv.Start(gctx) })
			g.Go(func() error { return sup.Start(gctx) })
			if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
				return err
			}
			return nil
		},
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
