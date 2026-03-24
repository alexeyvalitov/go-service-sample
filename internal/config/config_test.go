package config

import (
	"errors"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	got, err := load([]string{"app"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.HTTPPort != 8081 {
		t.Fatalf("HTTPPort: got %d want %d", got.HTTPPort, 8081)
	}
	if got.LogLevel != "info" {
		t.Fatalf("LogLevel: got %q want %q", got.LogLevel, "info")
	}
	if got.MaxBodyBytes != 1<<20 {
		t.Fatalf("MaxBodyBytes: got %d want %d", got.MaxBodyBytes, int64(1<<20))
	}
	if got.ReadTimeout != 10*time.Second {
		t.Fatalf("ReadTimeout: got %s want %s", got.ReadTimeout, 10*time.Second)
	}
	if got.ShutdownTimeout != 5*time.Second {
		t.Fatalf("ShutdownTimeout: got %s want %s", got.ShutdownTimeout, 5*time.Second)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("MAX_BODY_BYTES", "12345")
	t.Setenv("READ_TIMEOUT", "3s")
	t.Setenv("LOG_LEVEL", "DeBuG")
	t.Setenv("DRAIN_WINDOW", " 150ms ")
	t.Setenv("WORKER_ENABLED", " 1 ")
	t.Setenv("WORKER_INTERVAL", " 250ms ")
	t.Setenv("EXTERNAL_BASE_URL", " https://example.com ")
	t.Setenv("EXTERNAL_TIMEOUT", " 150ms ")
	t.Setenv("EXTERNAL_SOFT", " 1 ")
	t.Setenv("READY_TIMEOUT", " 800ms ")

	got, err := load([]string{"app"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.HTTPPort != 9000 {
		t.Fatalf("HTTPPort: got %d want %d", got.HTTPPort, 9000)
	}
	if got.MaxBodyBytes != 12345 {
		t.Fatalf("MaxBodyBytes: got %d want %d", got.MaxBodyBytes, int64(12345))
	}
	if got.ReadTimeout != 3*time.Second {
		t.Fatalf("ReadTimeout: got %s want %s", got.ReadTimeout, 3*time.Second)
	}
	if got.LogLevel != "debug" {
		t.Fatalf("LogLevel: got %q want %q", got.LogLevel, "debug")
	}
	if got.DrainWindow != 150*time.Millisecond {
		t.Fatalf("DrainWindow: got %v want %v", got.DrainWindow, 150*time.Millisecond)
	}
	if !got.WorkerEnabled {
		t.Fatalf("WorkerEnabled: got %v want %v", got.WorkerEnabled, true)
	}
	if got.WorkerInterval != 250*time.Millisecond {
		t.Fatalf("WorkerInterval: got %v want %v", got.WorkerInterval, 250*time.Millisecond)
	}
	if got.ExternalBaseURL != "https://example.com" {
		t.Fatalf("ExternalBaseURL: got %q want %q", got.ExternalBaseURL, "https://example.com")
	}
	if got.ExternalTimeout != 150*time.Millisecond {
		t.Fatalf("ExternalTimeout: got %v want %v", got.ExternalTimeout, 150*time.Millisecond)
	}
	if !got.ExternalSoft {
		t.Fatalf("ExternalSoft: got %v want %v", got.ExternalSoft, true)
	}
	if got.ReadyTimeout != 800*time.Millisecond {
		t.Fatalf("ReadyTimeout: got %v want %v", got.ReadyTimeout, 800*time.Millisecond)
	}
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	t.Setenv("PORT", "9000")
	t.Setenv("MAX_BODY_BYTES", "100")
	t.Setenv("LOG_LEVEL", "info")

	got, err := load([]string{"app", "-http-port=7777", "-max-body-bytes=200", "-log-level=debug"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.HTTPPort != 7777 {
		t.Fatalf("HTTPPort: got %d want %d", got.HTTPPort, 7777)
	}
	if got.MaxBodyBytes != 200 {
		t.Fatalf("MaxBodyBytes: got %d want %d", got.MaxBodyBytes, int64(200))
	}
	if got.LogLevel != "debug" {
		t.Fatalf("LogLevel: got %q want %q", got.LogLevel, "debug")
	}
}

func TestLoad_PortFlagAlias_Works(t *testing.T) {
	t.Parallel()

	got, err := load([]string{"app", "-port=7777"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.HTTPPort != 7777 {
		t.Fatalf("HTTPPort: got %d want %d", got.HTTPPort, 7777)
	}
}

func TestLoad_HTTPPortAndPortFlags_Conflict(t *testing.T) {
	t.Parallel()

	_, err := load([]string{"app", "-http-port=1111", "-port=2222"})
	if err == nil {
		t.Fatalf("error: got nil want non-nil")
	}
	if !strings.Contains(err.Error(), "-http-port") || !strings.Contains(err.Error(), "-port") {
		t.Fatalf("error: got %q want mention both -http-port and -port", err.Error())
	}
}

func TestLoad_ConflictingPortEnvErrors(t *testing.T) {
	t.Setenv("HTTP_PORT", "1111")
	t.Setenv("PORT", "2222")

	_, err := load([]string{"app"})
	if err == nil {
		t.Fatalf("error: got nil want non-nil")
	}
	if !strings.Contains(err.Error(), "HTTP_PORT") || !strings.Contains(err.Error(), "PORT") {
		t.Fatalf("error: got %q want mention both HTTP_PORT and PORT", err.Error())
	}
}

func TestLoad_ConflictingBodyEnvErrors(t *testing.T) {
	t.Setenv("MAX_BODY_BYTES", "1")
	t.Setenv("BODY_LIMIT", "2")

	_, err := load([]string{"app"})
	if err == nil {
		t.Fatalf("error: got nil want non-nil")
	}
	if !strings.Contains(err.Error(), "MAX_BODY_BYTES") || !strings.Contains(err.Error(), "BODY_LIMIT") {
		t.Fatalf("error: got %q want mention both MAX_BODY_BYTES and BODY_LIMIT", err.Error())
	}
}

func TestLoad_Help_ReturnsErrHelp(t *testing.T) {
	t.Parallel()

	_, err := load([]string{"app", "-h"})
	if err == nil {
		t.Fatalf("error: got nil want non-nil")
	}
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("error: got %v want %v", err, flag.ErrHelp)
	}
}

func TestLoad_HTTPHostValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "empty_ok", args: []string{"app", "-http-host="}},
		{name: "hostname_ok", args: []string{"app", "-http-host=localhost"}},
		{name: "ipv4_ok", args: []string{"app", "-http-host=127.0.0.1"}},
		{name: "ipv6_ok", args: []string{"app", "-http-host=::1"}},
		{name: "ipv6_with_zone_ok", args: []string{"app", "-http-host=fe80::1%lo0"}},
		{name: "whitespace_rejected", args: []string{"app", "-http-host= local host "}, wantErr: true},
		{name: "url_rejected", args: []string{"app", "-http-host=http://localhost"}, wantErr: true},
		{name: "path_rejected", args: []string{"app", "-http-host=/tmp/sock"}, wantErr: true},
		{name: "bracketed_ipv6_rejected", args: []string{"app", "-http-host=[::1]"}, wantErr: true},
		{name: "host_port_rejected", args: []string{"app", "-http-host=localhost:8081"}, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := load(tt.args)
			if tt.wantErr && err == nil {
				t.Fatalf("error: got nil want non-nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("error: got %v want nil", err)
			}
		})
	}
}
