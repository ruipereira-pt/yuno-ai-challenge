package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/service"
)

type Handler struct {
	health *service.HealthService
	stream StreamConfig
}

type StreamConfig struct {
	Enabled       bool
	MaxFrameBytes int64
	ReadTimeout   time.Duration
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int
}

func NewHandler(health *service.HealthService) *Handler {
	return &Handler{
		health: health,
		stream: StreamConfig{
			Enabled:       false,
			MaxFrameBytes: 1024 * 1024,
			ReadTimeout:   30 * time.Second,
			BatchSize:     50,
			FlushInterval: 1 * time.Second,
			QueueSize:     1000,
		},
	}
}

func (h *Handler) WithStreamConfig(cfg StreamConfig) *Handler {
	if cfg.MaxFrameBytes <= 0 {
		cfg.MaxFrameBytes = h.stream.MaxFrameBytes
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = h.stream.ReadTimeout
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = h.stream.BatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = h.stream.FlushInterval
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = h.stream.QueueSize
	}
	h.stream = cfg
	return h
}

func (h *Handler) PostEventsBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBatchBodyBytes)

	var req model.BatchIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorPayload("payload_too_large", "request body exceeds size limit", map[string]string{"field": "events"}))
			return
		}
		writeJSON(w, http.StatusBadRequest, errorPayload("invalid_request", "invalid JSON payload", map[string]string{"field": "events"}))
		return
	}

	writeJSON(w, http.StatusOK, h.health.IngestBatch(req))
}

func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	psp := r.URL.Query().Get("psp")
	writeJSON(w, http.StatusOK, h.health.GetHealth(psp))
}

func (h *Handler) GetAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	psp := r.URL.Query().Get("psp")
	activeOnlyParam := r.URL.Query().Get("active_only")
	activeOnly := false
	if activeOnlyParam != "" {
		parsed, err := strconv.ParseBool(activeOnlyParam)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorPayload("invalid_request", "active_only must be a boolean", map[string]string{"field": "active_only"}))
			return
		}
		activeOnly = parsed
	}
	from, to, apiErr := parseRange(r)
	if apiErr != nil {
		writeJSON(w, apiErr.StatusCode, errorPayload(apiErr.Code, apiErr.Message, apiErr.Details))
		return
	}
	writeJSON(w, http.StatusOK, h.health.GetAlerts(psp, from, to, activeOnly))
}

func (h *Handler) GetComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	from, to, apiErr := parseRange(r)
	if apiErr != nil {
		writeJSON(w, apiErr.StatusCode, errorPayload(apiErr.Code, apiErr.Message, apiErr.Details))
		return
	}
	writeJSON(w, http.StatusOK, h.health.GetComparison(from, to))
}

func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	if _, err := os.Stat("docs/openapi.yaml"); err != nil {
		writeJSON(w, http.StatusNotFound, errorPayload("not_found", "openapi spec file not found", map[string]string{"path": "docs/openapi.yaml"}))
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	http.ServeFile(w, r, "docs/openapi.yaml")
}

func (h *Handler) GetDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>PSP API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.ui = SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis]
      });
    </script>
  </body>
</html>`))
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

const maxBatchBodyBytes int64 = 5 << 20 // 5MB

var errFromNotBeforeTo = errors.New("from must be before to")

type apiError struct {
	StatusCode int
	Code       string
	Message    string
	Details    map[string]string
}

func parseRange(r *http.Request) (time.Time, time.Time, *apiError) {
	now := time.Now().UTC()
	fromParam := r.URL.Query().Get("from")
	toParam := r.URL.Query().Get("to")

	if fromParam == "" && toParam == "" {
		return now.Add(-60 * time.Minute), now, nil
	}
	if fromParam == "" || toParam == "" {
		return time.Time{}, time.Time{}, &apiError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_range_format",
			Message:    "from and to must both be provided in RFC3339 format",
			Details:    map[string]string{"from": fromParam, "to": toParam},
		}
	}

	from, err := time.Parse(time.RFC3339, fromParam)
	if err != nil {
		return time.Time{}, time.Time{}, &apiError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_range_format",
			Message:    "from must be RFC3339",
			Details:    map[string]string{"field": "from"},
		}
	}
	to, err := time.Parse(time.RFC3339, toParam)
	if err != nil {
		return time.Time{}, time.Time{}, &apiError{
			StatusCode: http.StatusBadRequest,
			Code:       "invalid_range_format",
			Message:    "to must be RFC3339",
			Details:    map[string]string{"field": "to"},
		}
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, &apiError{
			StatusCode: http.StatusUnprocessableEntity,
			Code:       "invalid_range_bounds",
			Message:    errFromNotBeforeTo.Error(),
			Details:    map[string]string{"from": fromParam, "to": toParam},
		}
	}
	return from.UTC(), to.UTC(), nil
}
