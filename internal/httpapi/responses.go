package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

type EchoRequest struct {
	Message string `json:"message"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type ReadyzResponse struct {
	Status string `json:"status"`
	Failed string `json:"failed,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("req_id=%s write response failed: %v", GetRequestID(r), err)
	}
}

func WriteJSONError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	WriteJSON(w, r, status, ErrorResponse{Error: msg})
}
