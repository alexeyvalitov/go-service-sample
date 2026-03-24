package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
	"github.com/alexeyvalitov/go-service-sample/internal/logging"
)

func TestEchoHandler(t *testing.T) {
	t.Parallel()

	cfg := config.Config{MaxBodyBytes: 1 << 20}
	logger := logging.New("info")

	tests := []struct {
		name        string
		method      string
		contentType string
		body        []byte
		wantStatus  int
		wantCT      string
		wantEcho    EchoRequest
		wantErr     string
	}{
		{name: "ok", method: http.MethodPost, contentType: "application/json", body: []byte(`{"message":"hello"}`), wantStatus: http.StatusOK, wantCT: "application/json; charset=utf-8", wantEcho: EchoRequest{Message: "hello"}},
		{name: "ok_allows_json_with_charset", method: http.MethodPost, contentType: "application/json; charset=utf-8", body: []byte(`{"message":"hello"}`), wantStatus: http.StatusOK, wantCT: "application/json; charset=utf-8", wantEcho: EchoRequest{Message: "hello"}},
		{name: "wrong_method", method: http.MethodGet, contentType: "application/json", body: []byte(`{"message":"hello"}`), wantStatus: http.StatusMethodNotAllowed, wantCT: "application/json; charset=utf-8", wantErr: "method not allowed"},
		{name: "wrong_content_type", method: http.MethodPost, contentType: "text/plain", body: []byte(`{"message":"hello"}`), wantStatus: http.StatusUnsupportedMediaType, wantCT: "application/json; charset=utf-8", wantErr: "content-type must be application/json"},
		{name: "invalid_json", method: http.MethodPost, contentType: "application/json", body: []byte(`{"message":`), wantStatus: http.StatusBadRequest, wantCT: "application/json; charset=utf-8", wantErr: "invalid json"},
		{name: "empty_message", method: http.MethodPost, contentType: "application/json", body: []byte(`{}`), wantStatus: http.StatusBadRequest, wantCT: "application/json; charset=utf-8", wantErr: "message is required"},
		{name: "body_too_large", method: http.MethodPost, contentType: "application/json", body: append([]byte(`{"message":"`), append(bytes.Repeat([]byte("a"), (1<<20)+1), []byte(`"}`)...)...), wantStatus: http.StatusRequestEntityTooLarge, wantCT: "application/json; charset=utf-8", wantErr: "body too large"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, "/api/v1/echo", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)

			rec := httptest.NewRecorder()
			EchoHandler(cfg, logger)(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			if got, want := res.StatusCode, tt.wantStatus; got != want {
				t.Fatalf("status: got %d want %d", got, want)
			}
			if got, want := res.Header.Get("Content-Type"), tt.wantCT; got != want {
				t.Fatalf("Content-Type: got %q want %q", got, want)
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("read response body: %v", err)
			}

			if tt.wantErr != "" {
				var got ErrorResponse
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("decode error response: %v; body=%q", err, string(body))
				}
				if got.Error != tt.wantErr {
					t.Fatalf("error: got %q want %q", got.Error, tt.wantErr)
				}
				return
			}

			var got EchoRequest
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode echo response: %v; body=%q", err, string(body))
			}
			if got != tt.wantEcho {
				t.Fatalf("response: got %+v want %+v", got, tt.wantEcho)
			}
		})
	}
}
