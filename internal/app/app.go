package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
	"github.com/alexeyvalitov/go-service-sample/internal/httpapi"
	"github.com/alexeyvalitov/go-service-sample/internal/logging"
	"github.com/alexeyvalitov/go-service-sample/internal/users"
)

type App struct {
	cfg    config.Config
	logger *logging.Logger

	store *users.Store
	mux   *http.ServeMux
	srv   *http.Server

	readiness []httpapi.ReadinessCheck
	external  *ExternalHTTPDependency

	shuttingDown atomic.Bool

	workersMu sync.Mutex

	workersCtx    context.Context
	cancelWorkers context.CancelFunc
	workersWG     sync.WaitGroup
	workerTicks   atomic.Uint64

	workersStopOnce sync.Once
	workersStopped  chan struct{}
}

func New(cfg config.Config) (*App, error) {
	logger := logging.New(cfg.LogLevel)
	store := users.NewStore()
	mux := http.NewServeMux()

	a := &App{
		cfg:    cfg,
		logger: logger,
		store:  store,
		mux:    mux,
	}

	if err := a.initReadiness(); err != nil {
		return nil, err
	}
	a.registerRoutes()
	a.buildServer()
	return a, nil
}

func (a *App) registerRoutes() {
	httpapi.RegisterRoutes(
		a.mux,
		a.cfg,
		a.logger,
		a.store,
		httpapi.ReadyHandler(a.logger, a.cfg.ReadyTimeout, &a.shuttingDown, a.readiness),
	)
}

func (a *App) buildServer() {
	handler := httpapi.RequestIDMiddleware(
		httpapi.LoggingMiddleware(
			a.logger,
			httpapi.RecoveryMiddleware(a.logger, a.mux),
		),
	)

	a.srv = &http.Server{
		Addr:              a.cfg.HTTPAddr(),
		Handler:           handler,
		ReadHeaderTimeout: a.cfg.ReadHeaderTimeout,
		ReadTimeout:       a.cfg.ReadTimeout,
		WriteTimeout:      a.cfg.WriteTimeout,
		IdleTimeout:       a.cfg.IdleTimeout,
	}
}

func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.srv.Addr)
	if err != nil {
		return err
	}
	return a.RunWithListener(ctx, ln)
}

func (a *App) RunWithListener(ctx context.Context, ln net.Listener) error {
	a.startWorkers(ctx)
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		a.stopWorkers(stopCtx)
	}()

	errCh := make(chan error, 1)
	go func() {
		a.logger.Infof("listening on %s (log_level=%s)", ln.Addr().String(), a.cfg.LogLevel)
		err := a.srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		select {
		case errCh <- err:
		default:
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		a.shuttingDown.Store(true)
		a.logger.Infof("shutdown signal received")
		a.logger.Infof("readiness set to not_ready (shutdown started)")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	if a.cfg.DrainWindow > 0 {
		a.logger.Infof("draining for %s before shutdown...", a.cfg.DrainWindow)
		timer := time.NewTimer(a.cfg.DrainWindow)
		select {
		case <-timer.C:
			a.logger.Infof("drain window finished, proceeding with shutdown")
		case <-shutdownCtx.Done():
			a.logger.Infof("drain window interrupted by shutdown timeout, proceeding with shutdown")
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}

	a.logger.Infof("shutting down...")
	a.stopWorkers(shutdownCtx)
	if err := a.srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := a.cleanup(); err != nil {
		a.logger.Warnf("cleanup failed: %v", err)
	}

	var err error
	select {
	case err = <-errCh:
	case <-shutdownCtx.Done():
		a.logger.Warnf("server shutdown returned, but ListenAndServe did not exit before timeout: %v", shutdownCtx.Err())
		err = nil
	}

	if err == nil {
		a.logger.Infof("server stopped gracefully")
	} else {
		a.logger.Errorf("server stopped with error: %v", err)
	}
	return err
}

func (a *App) startWorkers(parent context.Context) {
	if !a.cfg.WorkerEnabled {
		return
	}

	a.workersMu.Lock()
	defer a.workersMu.Unlock()
	if a.workersStopped != nil {
		return
	}

	a.workersStopOnce = sync.Once{}
	a.workersCtx, a.cancelWorkers = context.WithCancel(parent)
	a.workersStopped = make(chan struct{})
	a.workersWG.Add(1)
	go func() {
		defer a.workersWG.Done()

		ticker := time.NewTicker(a.cfg.WorkerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-a.workersCtx.Done():
				return
			case <-ticker.C:
				n := a.workerTicks.Add(1)
				if n%100 == 0 {
					a.logger.Debugf("worker tick: n=%d", n)
				}
			}
		}
	}()
}

func (a *App) stopWorkers(ctx context.Context) {
	a.workersMu.Lock()
	cancel := a.cancelWorkers
	stopped := a.workersStopped
	a.workersMu.Unlock()

	if cancel == nil || stopped == nil {
		return
	}

	a.workersStopOnce.Do(func() {
		cancel()
		go func() {
			a.workersWG.Wait()
			close(stopped)

			a.workersMu.Lock()
			a.cancelWorkers = nil
			a.workersStopped = nil
			a.workersCtx = nil
			a.workersMu.Unlock()
		}()
	})

	select {
	case <-stopped:
	case <-ctx.Done():
		a.logger.Warnf("workers did not stop before timeout: %v", ctx.Err())
	}
}

func (a *App) initReadiness() error {
	if a.cfg.ExternalBaseURL == "" {
		return nil
	}

	dep, err := NewExternalHTTPDependency(a.cfg.ExternalBaseURL, a.cfg.ExternalTimeout)
	if err != nil {
		if !a.cfg.ExternalSoft {
			return err
		}
		a.logger.Warnf("external dependency init failed (soft, ignoring): %v", err)
		return nil
	}
	a.external = dep
	a.readiness = append(a.readiness, httpapi.ReadinessCheck{
		Name: "external_http",
		Hard: !a.cfg.ExternalSoft,
		Fn:   dep.Check,
	})
	return nil
}

func (a *App) cleanup() error {
	if a.external != nil {
		return a.external.Cleanup()
	}
	return nil
}

func (a *App) WorkerTicks() uint64 {
	return a.workerTicks.Load()
}

func (a *App) ShuttingDown() bool {
	return a.shuttingDown.Load()
}
