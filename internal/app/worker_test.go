package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
)

func TestWorker_TicksAndStopsOnShutdown(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	cfg := config.Config{
		LogLevel:          "debug",
		ReadHeaderTimeout: 200 * time.Millisecond,
		ReadTimeout:       200 * time.Millisecond,
		WriteTimeout:      200 * time.Millisecond,
		IdleTimeout:       200 * time.Millisecond,
		ShutdownTimeout:   2 * time.Second,
		MaxBodyBytes:      1 << 20,
		WorkerEnabled:     true,
		WorkerInterval:    10 * time.Millisecond,
		ReadyTimeout:      200 * time.Millisecond,
		ExternalSoft:      true,
		ExternalTimeout:   50 * time.Millisecond,
	}

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- app.RunWithListener(runCtx, ln)
	}()

	var stopOnce sync.Once
	stopAndWait := func() {
		stopOnce.Do(func() {
			cancelRun()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("RunWithListener: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("timeout waiting for RunWithListener to return")
			}
		})
	}
	t.Cleanup(stopAndWait)

	baseURL := fmt.Sprintf("http://%s", ln.Addr().String())
	client := &http.Client{Timeout: 200 * time.Millisecond}

	healthCtx, cancelHealth := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHealth()
	for {
		req, err := http.NewRequestWithContext(healthCtx, http.MethodGet, baseURL+"/healthz", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		res, err := client.Do(req)
		if res != nil {
			_ = res.Body.Close()
		}
		if err == nil && res.StatusCode == http.StatusOK {
			break
		}
		select {
		case <-healthCtx.Done():
			t.Fatalf("server did not become healthy before deadline: %v", healthCtx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}

	tickCtx, cancelTick := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelTick()
	for {
		if app.WorkerTicks() >= 2 {
			break
		}
		select {
		case <-tickCtx.Done():
			t.Fatalf("worker did not tick before deadline")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopAndWait()

	after := app.WorkerTicks()
	stableCtx, cancelStable := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelStable()
	for {
		if got := app.WorkerTicks(); got != after {
			t.Fatalf("worker ticks after shutdown: got %d want %d", got, after)
		}
		select {
		case <-stableCtx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}
