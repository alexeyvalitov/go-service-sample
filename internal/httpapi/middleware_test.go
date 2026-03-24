package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexeyvalitov/go-service-sample/internal/logging"
)

func TestRecoveryMiddleware_PanicBecomes500JSON(t *testing.T) {
	t.Parallel()

	logger := logging.New("info")
	h := RecoveryMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusInternalServerError)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type: got %q want %q", got, "application/json; charset=utf-8")
	}

	var payload ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	if payload.Error != "internal server error" {
		t.Fatalf("error: got %q want %q", payload.Error, "internal server error")
	}
}

func TestRequestIDMiddleware_GeneratesAndReturnsHeaderAndContext(t *testing.T) {
	t.Parallel()

	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := GetRequestID(r); id == "" {
			t.Fatalf("expected request id in context, got empty")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusNoContent)
	}
	if id := res.Header.Get(RequestIDHeader); id == "" {
		t.Fatalf("expected %s header in response", RequestIDHeader)
	}
}

func TestRequestIDMiddleware_UsesIncomingHeaderIfValid(t *testing.T) {
	t.Parallel()

	const incoming = "abc123-DEF_456"

	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetRequestID(r); got != incoming {
			t.Fatalf("expected request id %q in context, got %q", incoming, got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(RequestIDHeader, incoming)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if got := res.Header.Get(RequestIDHeader); got != incoming {
		t.Fatalf("%s: got %q want %q", RequestIDHeader, got, incoming)
	}
}

func TestStatusRecorder_DefaultStatusIs200AfterWrite(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr}

	if _, err := rec.Write([]byte("ok")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.status != http.StatusOK {
		t.Fatalf("status: got %d want %d", rec.status, http.StatusOK)
	}
}

func TestStatusRecorder_RespectsWriteHeader(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rr}

	rec.WriteHeader(http.StatusCreated)
	if _, err := io.WriteString(rec, "x"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.status != http.StatusCreated {
		t.Fatalf("status: got %d want %d", rec.status, http.StatusCreated)
	}
}
