package api

import (
	"encoding/json"
	"net/http"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", h.GetStatus)
	mux.HandleFunc("/docs", h.GetDocs)
	mux.HandleFunc("/openapi.yaml", h.GetOpenAPISpec)
	mux.HandleFunc("/events/batch", h.PostEventsBatch)
	mux.HandleFunc("/events/stream", h.GetEventsStream)
	mux.HandleFunc("/health", h.GetHealth)
	mux.HandleFunc("/psps", h.GetHealth)
	mux.HandleFunc("/alerts", h.GetAlerts)
	mux.HandleFunc("/comparison", h.GetComparison)
	return loggingMiddleware(mux)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorPayload(code string, message string, details map[string]string) model.ErrorResponse {
	var resp model.ErrorResponse
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Details = details
	return resp
}
