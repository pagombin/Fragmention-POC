// Package supervisor coordinates the lifecycle of every background service
// (collector, loader, deleter, compact, workload). It owns the shared context
// used to cancel the entire application on SIGINT/SIGTERM and ensures each
// service is drained cleanly within the configured shutdown timeout.
package supervisor

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// Service is any long-running goroutine managed by the supervisor. Run must
// block until ctx is cancelled or a fatal error is encountered; it must not
// panic.
type Service interface {
	Name() string
	Run(ctx context.Context) error
}

// Supervisor starts and stops registered Services in a single errgroup.
type Supervisor struct {
	logger   zerolog.Logger
	services []Service
	mu       sync.Mutex
}

// New constructs a Supervisor. The logger is used to annotate service-level
// events; each service derives its own child logger internally.
func New(logger zerolog.Logger) *Supervisor {
	return &Supervisor{logger: logger.With().Str("service", "supervisor").Logger()}
}

// Register adds a Service. Registration order is preserved, which matters for
// services that assume earlier ones are initialized (the collector should run
// before loader/deleter).
func (s *Supervisor) Register(svc Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.services = append(s.services, svc)
}

// Start launches every registered service and returns once ctx is cancelled
// or any service returns an error other than context.Canceled. The returned
// error is the first non-nil error seen.
func (s *Supervisor) Start(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	s.mu.Lock()
	names := make([]string, 0, len(s.services))
	for _, svc := range s.services {
		names = append(names, svc.Name())
		svc := svc
		g.Go(func() error {
			logger := s.logger.With().Str("component", svc.Name()).Logger()
			logger.Info().Msg("service starting")
			err := svc.Run(gctx)
			switch {
			case err == nil, errors.Is(err, context.Canceled):
				logger.Info().Msg("service stopped")
				return nil
			default:
				logger.Error().Err(err).Msg("service exited with error")
				return err
			}
		})
	}
	s.mu.Unlock()
	s.logger.Info().Strs("services", names).Msg("supervisor started")
	return g.Wait()
}

// DrainTimeout is the default time the supervisor waits for in-flight work to
// finish after cancellation before returning.
const DrainTimeout = 30 * time.Second
