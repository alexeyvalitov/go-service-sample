package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
	"github.com/alexeyvalitov/go-service-sample/internal/httpapi"
)

func TestShutdown_DrainsAndFinishesInFlightRequest(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	cfg := config.Config{
		LogLevel:          "info",
		ReadHeaderTimeout: 200 * time.Millisecond,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       200 * time.Millisecond,
		ShutdownTimeout:   2 * time.Second,
		DrainWindow:       500 * time.Millisecond,
		MaxBodyBytes:      1 << 20,
		WorkerEnabled:     false,
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
	var stopErr error
	stopAndWait := func() {
		stopOnce.Do(func() {
			cancelRun()
			select {
			case stopErr = <-errCh:
			case <-time.After(3 * time.Second):
				stopErr = fmt.Errorf("timeout waiting for RunWithListener to return")
			}
		})
		if stopErr != nil {
			t.Fatalf("RunWithListener: %v", stopErr)
		}
	}
	t.Cleanup(stopAndWait)

	baseURL := fmt.Sprintf("http://%s", ln.Addr().String())
	client := &http.Client{Timeout: 3 * time.Second}

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

	sleepReq, err := http.NewRequest(http.MethodGet, baseURL+"/debug/sleep?d=300ms&flush=1", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	sleepRes, err := client.Do(sleepReq)
	if err != nil {
		t.Fatalf("sleep request: %v", err)
	}
	defer sleepRes.Body.Close()
	if sleepRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(sleepRes.Body)
		t.Fatalf("sleep status: got %d want %d body=%q", sleepRes.StatusCode, http.StatusOK, string(b))
	}
	br := bufio.NewReader(sleepRes.Body)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read started line: %v", err)
	}
	if line != "started\n" {
		t.Fatalf("started line: got %q want %q", line, "started\n")
	}

	cancelRun()

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancelDrain()
	for !app.ShuttingDown() {
		select {
		case <-drainCtx.Done():
			t.Fatalf("app did not enter shuttingDown before deadline")
		case <-time.After(5 * time.Millisecond):
		}
	}

	readyReq, err := http.NewRequestWithContext(drainCtx, http.MethodGet, baseURL+"/readyz", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	readyRes, err := client.Do(readyReq)
	if err != nil {
		t.Fatalf("readyz request: %v", err)
	}
	defer readyRes.Body.Close()
	if readyRes.StatusCode != http.StatusServiceUnavailable {
		b, _ := io.ReadAll(readyRes.Body)
		t.Fatalf("readyz status: got %d want %d body=%q", readyRes.StatusCode, http.StatusServiceUnavailable, string(b))
	}
	var rr httpapi.ReadyzResponse
	if err := json.NewDecoder(readyRes.Body).Decode(&rr); err != nil {
		t.Fatalf("decode readyz response: %v", err)
	}
	if rr.Status != "not_ready" {
		t.Fatalf("readyz status field: got %q want %q", rr.Status, "not_ready")
	}
	if rr.Failed != "shutdown" {
		t.Fatalf("readyz failed field: got %q want %q", rr.Failed, "shutdown")
	}

	rest, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("read remaining sleep body: %v", err)
	}
	if !strings.Contains(string(rest), "done\n") {
		t.Fatalf("sleep body: expected %q, got %q", "done\\n", string(rest))
	}

	stopAndWait()
}
