package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPHost string
	HTTPPort int

	LogLevel string

	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	DrainWindow       time.Duration

	MaxBodyBytes int64

	// Optional background worker used to demonstrate lifecycle management.
	WorkerEnabled  bool
	WorkerInterval time.Duration

	// Optional external dependency probed by /readyz.
	// If ExternalBaseURL is set, /readyz will check it (GET <base>/healthz).
	ExternalBaseURL string
	ExternalTimeout time.Duration
	ExternalSoft    bool

	// /readyz should be quick and bounded; this is an overall deadline for running readiness checks.
	ReadyTimeout time.Duration
}

func (c Config) HTTPAddr() string {
	// Handles IPv6 correctly (adds brackets), and also produces ":<port>" for empty host.
	return net.JoinHostPort(c.HTTPHost, strconv.Itoa(c.HTTPPort))
}

func (c Config) Validate() error {
	// Minimal sanity checks for host. We intentionally don't "validate" DNS here.
	// Goal: catch obvious config mistakes early, without false positives.
	if strings.TrimSpace(c.HTTPHost) != c.HTTPHost {
		return fmt.Errorf("http-host must not have leading/trailing spaces, got %q", c.HTTPHost)
	}
	if strings.ContainsAny(c.HTTPHost, " \t\r\n") {
		return fmt.Errorf("http-host must not contain whitespace, got %q", c.HTTPHost)
	}
	if strings.Contains(c.HTTPHost, "://") || strings.Contains(c.HTTPHost, "/") {
		return fmt.Errorf("http-host must be a host (not a URL/path), got %q", c.HTTPHost)
	}
	if strings.HasPrefix(c.HTTPHost, "[") || strings.HasSuffix(c.HTTPHost, "]") {
		return fmt.Errorf("http-host must not include brackets; pass IPv6 as ::1 (not [::1]), got %q", c.HTTPHost)
	}
	if c.HTTPHost != "" {
		// Catch common mistake: putting "host:port" into host.
		// If it contains ':', it must be an IPv6 literal (optionally with zone).
		hostNoZone := strings.SplitN(c.HTTPHost, "%", 2)[0]
		if strings.Contains(c.HTTPHost, ":") && net.ParseIP(hostNoZone) == nil {
			return fmt.Errorf("http-host must be a hostname or IP without port, got %q", c.HTTPHost)
		}
	}

	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("port must be in range 1..65535, got %d", c.HTTPPort)
	}

	switch c.LogLevel {
	case "debug", "info":
	default:
		return fmt.Errorf("invalid log-level %q (expected debug or info)", c.LogLevel)
	}

	if c.MaxBodyBytes <= 0 {
		return fmt.Errorf("max-body-bytes must be > 0, got %d", c.MaxBodyBytes)
	}
	if c.ReadHeaderTimeout <= 0 || c.ReadTimeout <= 0 || c.WriteTimeout <= 0 || c.IdleTimeout <= 0 {
		return errors.New("timeouts must be > 0")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown-timeout must be > 0")
	}
	if c.DrainWindow < 0 {
		return errors.New("drain-window must be >= 0")
	}
	if c.DrainWindow > c.ShutdownTimeout {
		return fmt.Errorf("drain-window must be <= shutdown-timeout (got %s > %s)", c.DrainWindow, c.ShutdownTimeout)
	}

	if c.WorkerEnabled && c.WorkerInterval <= 0 {
		return errors.New("worker-interval must be > 0 when worker-enabled is true")
	}

	if c.ExternalTimeout <= 0 {
		return errors.New("external-timeout must be > 0")
	}
	if c.ReadyTimeout <= 0 {
		return errors.New("ready-timeout must be > 0")
	}
	if strings.TrimSpace(c.ExternalBaseURL) != c.ExternalBaseURL {
		return fmt.Errorf("external-base-url must not have leading/trailing spaces, got %q", c.ExternalBaseURL)
	}
	if c.ExternalBaseURL != "" {
		if c.ExternalTimeout > c.ReadyTimeout {
			return fmt.Errorf("external-timeout must be <= ready-timeout (got %s > %s)", c.ExternalTimeout, c.ReadyTimeout)
		}
		u, err := url.Parse(c.ExternalBaseURL)
		if err != nil {
			return fmt.Errorf("external-base-url: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("external-base-url must include scheme and host, got %q", c.ExternalBaseURL)
		}
		if u.Fragment != "" {
			return fmt.Errorf("external-base-url must not include fragment, got %q", c.ExternalBaseURL)
		}
	}

	return nil
}

func Load() (Config, error) {
	return load(os.Args)
}

func load(args []string) (Config, error) {
	cfg := Config{
		HTTPHost: "",
		HTTPPort: 8081,

		LogLevel: "info",

		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   5 * time.Second,
		DrainWindow:       0,

		MaxBodyBytes: 1 << 20,

		WorkerEnabled:  false,
		WorkerInterval: 1 * time.Second,

		ExternalBaseURL: "",
		ExternalTimeout: 250 * time.Millisecond,
		ExternalSoft:    false,
		ReadyTimeout:    1 * time.Second,
	}

	if v := strings.TrimSpace(os.Getenv("HTTP_HOST")); v != "" {
		cfg.HTTPHost = v
	}
	if v := strings.TrimSpace(os.Getenv("LOG_LEVEL")); v != "" {
		cfg.LogLevel = strings.ToLower(v)
	}
	httpPortEnv := strings.TrimSpace(os.Getenv("HTTP_PORT"))
	portEnv := strings.TrimSpace(os.Getenv("PORT"))
	if httpPortEnv != "" && portEnv != "" && httpPortEnv != portEnv {
		return Config{}, fmt.Errorf("both HTTP_PORT=%q and PORT=%q are set; unset one to avoid ambiguity", httpPortEnv, portEnv)
	}
	if httpPortEnv != "" {
		p, err := parsePort(httpPortEnv)
		if err != nil {
			return Config{}, fmt.Errorf("HTTP_PORT: %w", err)
		}
		cfg.HTTPPort = p
	} else if portEnv != "" {
		p, err := parsePort(portEnv)
		if err != nil {
			return Config{}, fmt.Errorf("PORT: %w", err)
		}
		cfg.HTTPPort = p
	}

	maxBodyEnv := strings.TrimSpace(os.Getenv("MAX_BODY_BYTES"))
	bodyLimitEnv := strings.TrimSpace(os.Getenv("BODY_LIMIT"))
	if maxBodyEnv != "" && bodyLimitEnv != "" && maxBodyEnv != bodyLimitEnv {
		return Config{}, fmt.Errorf("both MAX_BODY_BYTES=%q and BODY_LIMIT=%q are set; unset one to avoid ambiguity", maxBodyEnv, bodyLimitEnv)
	}
	if maxBodyEnv != "" {
		n, err := strconv.ParseInt(maxBodyEnv, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_BODY_BYTES: parse int: %w", err)
		}
		cfg.MaxBodyBytes = n
	} else if bodyLimitEnv != "" {
		n, err := strconv.ParseInt(bodyLimitEnv, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("BODY_LIMIT: parse int: %w", err)
		}
		cfg.MaxBodyBytes = n
	}

	if v := strings.TrimSpace(os.Getenv("READ_HEADER_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("READ_HEADER_TIMEOUT: %w", err)
		}
		cfg.ReadHeaderTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("READ_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("READ_TIMEOUT: %w", err)
		}
		cfg.ReadTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("WRITE_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("WRITE_TIMEOUT: %w", err)
		}
		cfg.WriteTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("IDLE_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("IDLE_TIMEOUT: %w", err)
		}
		cfg.IdleTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("SHUTDOWN_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("DRAIN_WINDOW")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("DRAIN_WINDOW: %w", err)
		}
		cfg.DrainWindow = d
	}

	if v := strings.TrimSpace(os.Getenv("WORKER_ENABLED")); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("WORKER_ENABLED: %w", err)
		}
		cfg.WorkerEnabled = b
	}
	if v := strings.TrimSpace(os.Getenv("WORKER_INTERVAL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("WORKER_INTERVAL: %w", err)
		}
		cfg.WorkerInterval = d
	}

	if v := strings.TrimSpace(os.Getenv("EXTERNAL_BASE_URL")); v != "" {
		cfg.ExternalBaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("EXTERNAL_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("EXTERNAL_TIMEOUT: %w", err)
		}
		cfg.ExternalTimeout = d
	}
	if v := strings.TrimSpace(os.Getenv("EXTERNAL_SOFT")); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("EXTERNAL_SOFT: %w", err)
		}
		cfg.ExternalSoft = b
	}
	if v := strings.TrimSpace(os.Getenv("READY_TIMEOUT")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("READY_TIMEOUT: %w", err)
		}
		cfg.ReadyTimeout = d
	}

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.HTTPHost, "http-host", cfg.HTTPHost, "HTTP listen host (empty means all interfaces)")
	var httpPortFlag int
	var portFlag int
	fs.IntVar(&httpPortFlag, "http-port", cfg.HTTPPort, "HTTP listen port")
	fs.IntVar(&portFlag, "port", cfg.HTTPPort, "DEPRECATED: use -http-port (HTTP listen port)")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug or info")
	fs.Int64Var(&cfg.MaxBodyBytes, "max-body-bytes", cfg.MaxBodyBytes, "max request body size in bytes")
	fs.DurationVar(&cfg.ReadHeaderTimeout, "read-header-timeout", cfg.ReadHeaderTimeout, "HTTP server ReadHeaderTimeout")
	fs.DurationVar(&cfg.ReadTimeout, "read-timeout", cfg.ReadTimeout, "HTTP server ReadTimeout")
	fs.DurationVar(&cfg.WriteTimeout, "write-timeout", cfg.WriteTimeout, "HTTP server WriteTimeout")
	fs.DurationVar(&cfg.IdleTimeout, "idle-timeout", cfg.IdleTimeout, "HTTP server IdleTimeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "graceful shutdown timeout")
	fs.DurationVar(&cfg.DrainWindow, "drain-window", cfg.DrainWindow, "optional drain window before shutdown (time to let LB stop sending new requests)")
	fs.BoolVar(&cfg.WorkerEnabled, "worker-enabled", cfg.WorkerEnabled, "enable background worker")
	fs.DurationVar(&cfg.WorkerInterval, "worker-interval", cfg.WorkerInterval, "background worker interval")
	fs.StringVar(&cfg.ExternalBaseURL, "external-base-url", cfg.ExternalBaseURL, "external dependency base URL (checked by /readyz)")
	fs.DurationVar(&cfg.ExternalTimeout, "external-timeout", cfg.ExternalTimeout, "external dependency timeout")
	fs.BoolVar(&cfg.ExternalSoft, "external-soft", cfg.ExternalSoft, "treat external dependency as soft (warn-only) readiness check")
	fs.DurationVar(&cfg.ReadyTimeout, "ready-timeout", cfg.ReadyTimeout, "overall /readyz timeout (deadline for running readiness checks)")

	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(os.Stderr)
			fs.PrintDefaults()
		}
		return Config{}, err
	}

	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	if seen["http-port"] && seen["port"] && httpPortFlag != portFlag {
		return Config{}, fmt.Errorf("both -http-port=%d and -port=%d are set; use only one", httpPortFlag, portFlag)
	}
	if seen["http-port"] {
		cfg.HTTPPort = httpPortFlag
	} else if seen["port"] {
		cfg.HTTPPort = portFlag
	}
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("parse int: %w", err)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port must be 1..65535, got %d", n)
	}
	return n, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "y", "yes", "on":
		return true, nil
	case "0", "f", "false", "n", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q (expected true/false/1/0)", s)
	}
}
