package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
	"github.com/alexeyvalitov/go-service-sample/internal/logging"
	"github.com/alexeyvalitov/go-service-sample/internal/users"
)

type ReadinessCheck struct {
	Name string
	Hard bool
	Fn   func(context.Context) error
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func RegisterRoutes(mux *http.ServeMux, cfg config.Config, logger *logging.Logger, store *users.Store, readyz http.HandlerFunc) {
	mux.HandleFunc("/ping", PingHandler(logger))
	mux.HandleFunc("/healthz", HealthHandler(logger))
	mux.HandleFunc("/readyz", readyz)
	mux.HandleFunc("/debug/sleep", DebugSleepHandler(logger))
	mux.HandleFunc("/api/v1/echo", EchoHandler(cfg, logger))
	mux.HandleFunc("/api/v1/users", UsersHandler(store, cfg))
	mux.HandleFunc("/api/v1/users/", UserHandler(store, cfg))
}

func EchoHandler(cfg config.Config, logger *logging.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debugf("req_id=%s request: %s %s %s ct=%q", GetRequestID(r), r.Method, r.URL.RequestURI(), r.Proto, r.Header.Get("Content-Type"))

		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			WriteJSONError(w, r, http.StatusUnsupportedMediaType, "content-type must be application/json")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				WriteJSONError(w, r, http.StatusRequestEntityTooLarge, "body too large")
				return
			}
			WriteJSONError(w, r, http.StatusBadRequest, "failed to read body")
			return
		}

		var req EchoRequest
		if err := json.Unmarshal(body, &req); err != nil {
			WriteJSONError(w, r, http.StatusBadRequest, "invalid json")
			return
		}

		if req.Message == "" {
			WriteJSONError(w, r, http.StatusBadRequest, "message is required")
			return
		}

		WriteJSON(w, r, http.StatusOK, req)
	}
}

func PingHandler(logger *logging.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debugf("req_id=%s request: %s %s %s host=%q remote=%q ua=%q accept=%q", GetRequestID(r), r.Method, r.URL.RequestURI(), r.Proto, r.Host, r.RemoteAddr, r.UserAgent(), r.Header.Get("Accept"))

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		logger.Debugf("req_id=%s response: status=%d content-type=%q", GetRequestID(r), http.StatusOK, w.Header().Get("Content-Type"))
		if _, err := io.WriteString(w, "pong"); err != nil {
			logger.Errorf("req_id=%s write response failed: %v", GetRequestID(r), err)
		}
	}
}

func HealthHandler(logger *logging.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debugf("req_id=%s request: %s %s %s host=%q remote=%q ua=%q accept=%q", GetRequestID(r), r.Method, r.URL.RequestURI(), r.Proto, r.Host, r.RemoteAddr, r.UserAgent(), r.Header.Get("Accept"))

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		logger.Debugf("req_id=%s response: status=%d content-type=%q", GetRequestID(r), http.StatusOK, "application/json; charset=utf-8")
		WriteJSON(w, r, http.StatusOK, HealthResponse{Status: "ok"})
	}
}

func DebugSleepHandler(logger *logging.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		d := time.Second
		if raw := strings.TrimSpace(r.URL.Query().Get("d")); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				WriteJSONError(w, r, http.StatusBadRequest, "invalid duration")
				return
			}
			d = parsed
		}
		if d < 0 {
			WriteJSONError(w, r, http.StatusBadRequest, "duration must be >= 0")
			return
		}
		if d > 10*time.Second {
			WriteJSONError(w, r, http.StatusBadRequest, "duration too large (max 10s)")
			return
		}

		if r.URL.Query().Get("flush") == "1" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "started\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}

		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "done\n")
		case <-r.Context().Done():
			logger.Infof("sleep handler cancelled: %v", r.Context().Err())
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "cancelled\n")
		}
	}
}

func UsersHandler(store *users.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			WriteJSON(w, r, http.StatusOK, store.List())
		case http.MethodPost:
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || mediaType != "application/json" {
				WriteJSONError(w, r, http.StatusUnsupportedMediaType, "content-type must be application/json")
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
			defer r.Body.Close()

			var req CreateUserRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					WriteJSONError(w, r, http.StatusRequestEntityTooLarge, "body too large")
					return
				}
				WriteJSONError(w, r, http.StatusBadRequest, "invalid json")
				return
			}

			if req.Name == "" {
				WriteJSONError(w, r, http.StatusBadRequest, "name is required")
				return
			}

			WriteJSON(w, r, http.StatusCreated, store.Create(req.Name))
		default:
			w.Header().Set("Allow", "GET, POST")
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func UserHandler(store *users.Store, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/users/" {
			UsersHandler(store, cfg)(w, r)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		id, rest, _ := strings.Cut(id, "/")
		if id == "" || rest != "" {
			WriteJSONError(w, r, http.StatusNotFound, "not found")
			return
		}

		switch r.Method {
		case http.MethodGet:
			u, err := store.Get(id)
			if errors.Is(err, users.ErrNotFound) {
				WriteJSONError(w, r, http.StatusNotFound, "user not found")
				return
			}
			WriteJSON(w, r, http.StatusOK, u)
		case http.MethodDelete:
			if err := store.Delete(id); errors.Is(err, users.ErrNotFound) {
				WriteJSONError(w, r, http.StatusNotFound, "user not found")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func ReadyHandler(logger *logging.Logger, readyTimeout time.Duration, shuttingDown *atomic.Bool, checks []ReadinessCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			WriteJSONError(w, r, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if shuttingDown != nil && shuttingDown.Load() {
			WriteJSON(w, r, http.StatusServiceUnavailable, ReadyzResponse{Status: "not_ready", Failed: "shutdown"})
			return
		}

		readyCtx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()
		for _, check := range checks {
			if err := check.Fn(readyCtx); err != nil {
				if check.Hard {
					logger.Warnf("readiness check failed: %s: %v", check.Name, err)
					WriteJSON(w, r, http.StatusServiceUnavailable, ReadyzResponse{Status: "not_ready", Failed: check.Name})
					return
				}
				logger.Infof("soft readiness check failed: %s: %v", check.Name, err)
			}
		}

		WriteJSON(w, r, http.StatusOK, ReadyzResponse{Status: "ready"})
	}
}
