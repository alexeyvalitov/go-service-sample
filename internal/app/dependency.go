package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type ExternalHTTPDependency struct {
	baseURL   string
	healthURL string
	timeout   time.Duration

	client    *http.Client
	transport *http.Transport
}

func NewExternalHTTPDependency(baseURL string, timeout time.Duration) (*ExternalHTTPDependency, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("base url must include scheme and host, got %q", baseURL)
	}

	healthURL, err := url.JoinPath(baseURL, "/healthz")
	if err != nil {
		return nil, fmt.Errorf("build health url: %w", err)
	}

	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("http.DefaultTransport is not *http.Transport")
	}
	tr := dt.Clone()
	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	return &ExternalHTTPDependency{
		baseURL:   baseURL,
		healthURL: healthURL,
		timeout:   timeout,
		client:    client,
		transport: tr,
	}, nil
}

func (d *ExternalHTTPDependency) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.healthURL, nil)
	if err != nil {
		return fmt.Errorf("external http check (%s): new request: %w", d.baseURL, err)
	}

	res, err := d.client.Do(req)
	if res != nil {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("external http check (%s): %w", d.baseURL, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("external http check (%s): unexpected status: %s", d.baseURL, res.Status)
	}
	return nil
}

func (d *ExternalHTTPDependency) Cleanup() error {
	if d.transport == nil {
		return nil
	}
	d.transport.CloseIdleConnections()
	return nil
}
