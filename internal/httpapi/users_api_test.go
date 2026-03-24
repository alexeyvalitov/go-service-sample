package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexeyvalitov/go-service-sample/internal/config"
	"github.com/alexeyvalitov/go-service-sample/internal/logging"
	"github.com/alexeyvalitov/go-service-sample/internal/users"
)

func newTestAPI() http.Handler {
	store := users.NewStore()
	cfg := config.Config{MaxBodyBytes: 1 << 20}
	logger := logging.New("info")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", UsersHandler(store, cfg))
	mux.HandleFunc("/api/v1/users/", UserHandler(store, cfg))

	return RequestIDMiddleware(
		LoggingMiddleware(logger,
			RecoveryMiddleware(logger, mux),
		),
	)
}

func TestUsersCRUDFlow(t *testing.T) {
	api := newTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"name":"Alice"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusCreated)
	}

	var created users.User
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("created.id: got empty want non-empty")
	}
	if created.Name != "Alice" {
		t.Fatalf("created.name: got %q want %q", created.Name, "Alice")
	}
	if created.CreatedAt.IsZero() {
		t.Fatalf("created.created_at: got zero want non-zero")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res = rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}

	var list []users.User
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatalf("decode user list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("users count: got %d want %d", len(list), 1)
	}
	if got := list[0].ID; got != created.ID {
		t.Fatalf("list[0].id: got %q want %q", got, created.ID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/users/"+created.ID, nil)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res = rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusOK)
	}

	var gotByID users.User
	if err := json.NewDecoder(res.Body).Decode(&gotByID); err != nil {
		t.Fatalf("decode get-by-id user: %v", err)
	}
	if got := gotByID.ID; got != created.ID {
		t.Fatalf("get-by-id.id: got %q want %q", got, created.ID)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/"+created.ID, nil)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res = rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusNoContent)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/users/"+created.ID, nil)
	rec = httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res = rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusNotFound)
	}
}

func TestUserNotFound(t *testing.T) {
	api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/doesnotexist", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusNotFound)
	}

	var payload ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error != "user not found" {
		t.Fatalf("error: got %q want %q", payload.Error, "user not found")
	}
}

func TestUsersCreate_BodyTooLargeBecomes413JSON(t *testing.T) {
	api := newTestAPI()

	huge := strings.Repeat("A", (1<<20)+1024)
	body := `{"name":"` + huge + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status: got %d want %d", res.StatusCode, http.StatusRequestEntityTooLarge)
	}

	var payload ErrorResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error != "body too large" {
		t.Fatalf("error: got %q want %q", payload.Error, "body too large")
	}
}
