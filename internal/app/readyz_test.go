package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
)

func startTestApp(t *testing.T, cfg config.Config) (baseURL string, stop func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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
	stop = func() {
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
	t.Cleanup(stop)

	baseURL = fmt.Sprintf("http://%s", ln.Addr().String())

	healthCtx, cancelHealth := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelHealth()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: 200 * time.Millisecond}
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
			return baseURL, stop
		}

		select {
		case <-healthCtx.Done():
			t.Fatalf("server did not become healthy before deadline: %v", healthCtx.Err())
		case <-ticker.C:
		}
	}
}

func baseTestConfig() config.Config {
	return config.Config{
		LogLevel:          "info",
		ReadHeaderTimeout: 200 * time.Millisecond,
		ReadTimeout:       200 * time.Millisecond,
		WriteTimeout:      200 * time.Millisecond,
		IdleTimeout:       200 * time.Millisecond,
		ShutdownTimeout:   2 * time.Second,
		MaxBodyBytes:      1 << 20,
		ExternalTimeout:   200 * time.Millisecond,
		ReadyTimeout:      1 * time.Second,
	}
}

func TestReadyz_NoDeps_IsReady(t *testing.T) {
	t.Parallel()

	cfg := baseTestConfig()
	baseURL, _ := startTestApp(t, cfg)

	res, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("get /readyz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
}

func TestReadyz_ExternalHard_OK_IsReady(t *testing.T) {
	t.Parallel()

	dep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(dep.Close)

	cfg := baseTestConfig()
	cfg.ExternalBaseURL = dep.URL
	cfg.ExternalSoft = false

	baseURL, _ := startTestApp(t, cfg)

	res, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("get /readyz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
}

func TestReadyz_ExternalHard_Fails_NotReady(t *testing.T) {
	t.Parallel()

	dep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(dep.Close)

	cfg := baseTestConfig()
	cfg.ExternalBaseURL = dep.URL
	cfg.ExternalSoft = false

	baseURL, _ := startTestApp(t, cfg)

	res, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("get /readyz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusServiceUnavailable)
	}
}

func TestReadyz_ExternalSoft_Fails_IsStillReady(t *testing.T) {
	t.Parallel()

	dep := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(dep.Close)

	cfg := baseTestConfig()
	cfg.ExternalBaseURL = dep.URL
	cfg.ExternalSoft = true

	baseURL, _ := startTestApp(t, cfg)

	res, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatalf("get /readyz: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}
}
