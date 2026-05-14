package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/service"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

func newTestRouter() http.Handler {
	windowStore := store.NewWindowStore()
	scorer := service.NewScorer()
	alerts := service.NewAlertEvaluator(service.AlertConfig{
		HealthThreshold:   60,
		ApprovalThreshold: 0.70,
		ErrorThreshold:    0.15,
		SustainedFor:      5 * time.Minute,
	}, scorer)
	healthSvc := service.NewHealthService(windowStore, scorer, alerts, 180*time.Minute, 2*time.Minute)
	return NewRouter(NewHandler(healthSvc))
}

func newTestRouterWithStreamEnabled() http.Handler {
	windowStore := store.NewWindowStore()
	scorer := service.NewScorer()
	alerts := service.NewAlertEvaluator(service.AlertConfig{
		HealthThreshold:   60,
		ApprovalThreshold: 0.70,
		ErrorThreshold:    0.15,
		SustainedFor:      5 * time.Minute,
	}, scorer)
	healthSvc := service.NewHealthService(windowStore, scorer, alerts, 180*time.Minute, 2*time.Minute)
	handler := NewHandler(healthSvc).WithStreamConfig(StreamConfig{Enabled: true})
	return NewRouter(handler)
}

func TestEventsBatchContract(t *testing.T) {
	router := newTestRouter()
	now := time.Now().UTC()
	body := model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-1",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 240,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-1",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusError,
				ResponseTimeMs: 900,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp model.BatchIngestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.AcceptedCount != 1 || resp.DuplicateCount != 1 || resp.RejectedCount != 0 {
		t.Fatalf("unexpected counts: %+v", resp)
	}
}

func TestEventsBatchPayloadTooLarge(t *testing.T) {
	router := newTestRouter()

	hugeID := strings.Repeat("x", int(maxBatchBodyBytes))
	body, _ := json.Marshal(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  hugeID,
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 123,
				Timestamp:      time.Now().UTC(),
			},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}

	var errResp model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code != "payload_too_large" {
		t.Fatalf("expected payload_too_large code, got %s", errResp.Error.Code)
	}
}

func TestHealthUnknownPSPReturnsEmpty(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/health?psp=UNKNOWN", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp model.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.PSPs) != 0 {
		t.Fatalf("expected empty psps, got %d", len(resp.PSPs))
	}
}

func TestComparisonRangeErrors(t *testing.T) {
	router := newTestRouter()

	req400 := httptest.NewRequest(http.MethodGet, "/comparison?from=bad&to=2026-05-14T10:00:00Z", nil)
	rec400 := httptest.NewRecorder()
	router.ServeHTTP(rec400, req400)
	if rec400.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec400.Code)
	}

	req422 := httptest.NewRequest(http.MethodGet, "/comparison?from=2026-05-14T10:00:00Z&to=2026-05-14T10:00:00Z", nil)
	rec422 := httptest.NewRecorder()
	router.ServeHTTP(rec422, req422)
	if rec422.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec422.Code)
	}

	var errResp model.ErrorResponse
	if err := json.Unmarshal(rec422.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error.Code == "" || errResp.Error.Message == "" {
		t.Fatalf("expected error envelope with code and message")
	}
	if errResp.Error.Code != "invalid_range_bounds" {
		t.Fatalf("expected error code invalid_range_bounds, got %s", errResp.Error.Code)
	}
	if errResp.Error.Details["from"] == "" || errResp.Error.Details["to"] == "" {
		t.Fatalf("expected from/to details in error payload")
	}
}

func TestStatusEndpoint(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/status", nil)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /status, got %d", postRec.Code)
	}
}

func TestPspsAliasMatchesHealthContract(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/psps", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp model.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
}

func TestAlertsEndpointContract(t *testing.T) {
	router := newTestRouter()

	// Success path (default range).
	okReq := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", okRec.Code)
	}
	var okResp model.AlertsResponse
	if err := json.Unmarshal(okRec.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("failed to decode alerts response: %v", err)
	}

	// Invalid format -> 400.
	badReq := httptest.NewRequest(http.MethodGet, "/alerts?from=bad&to=2026-05-14T10:00:00Z", nil)
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badRec.Code)
	}
	var badErr model.ErrorResponse
	if err := json.Unmarshal(badRec.Body.Bytes(), &badErr); err != nil {
		t.Fatalf("failed to decode bad request error response: %v", err)
	}
	if badErr.Error.Code != "invalid_range_format" {
		t.Fatalf("expected invalid_range_format, got %s", badErr.Error.Code)
	}
	if badErr.Error.Details["field"] != "from" {
		t.Fatalf("expected details.field=from, got %v", badErr.Error.Details)
	}

	invalidBoolReq := httptest.NewRequest(http.MethodGet, "/alerts?active_only=not-bool", nil)
	invalidBoolRec := httptest.NewRecorder()
	router.ServeHTTP(invalidBoolRec, invalidBoolReq)
	if invalidBoolRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid active_only, got %d", invalidBoolRec.Code)
	}

	// Invalid bounds -> 422.
	boundsReq := httptest.NewRequest(http.MethodGet, "/alerts?from=2026-05-14T10:00:00Z&to=2026-05-14T10:00:00Z", nil)
	boundsRec := httptest.NewRecorder()
	router.ServeHTTP(boundsRec, boundsReq)
	if boundsRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", boundsRec.Code)
	}
	var boundsErr model.ErrorResponse
	if err := json.Unmarshal(boundsRec.Body.Bytes(), &boundsErr); err != nil {
		t.Fatalf("failed to decode bounds error response: %v", err)
	}
	if boundsErr.Error.Code != "invalid_range_bounds" {
		t.Fatalf("expected invalid_range_bounds, got %s", boundsErr.Error.Code)
	}
}

func TestMethodConstraints(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestEventsStreamDisabledByDefault(t *testing.T) {
	router := newTestRouter()

	getReq := httptest.NewRequest(http.MethodGet, "/events/stream", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when stream is disabled, got %d", getRec.Code)
	}

	var notFound model.ErrorResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &notFound); err != nil {
		t.Fatalf("decode not found error response: %v", err)
	}
	if notFound.Error.Code != "not_found" {
		t.Fatalf("expected not_found code, got %s", notFound.Error.Code)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/events/stream", nil)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /events/stream, got %d", postRec.Code)
	}
}

func TestEventsStreamEnabledWebsocketHandshake(t *testing.T) {
	router := newTestRouterWithStreamEnabled()
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/events/stream"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("expected websocket handshake to succeed, got error: %v", err)
	}
	defer conn.Close()

	now := time.Now().UTC()
	event := model.TransactionEvent{
		TransactionID:  "ws-test-handshake-1",
		PSP:            "PSP_ALPHA",
		Status:         model.StatusApproved,
		ResponseTimeMs: 240,
		Timestamp:      now,
	}
	if err := conn.WriteJSON(event); err != nil {
		t.Fatalf("write websocket event: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var ack model.BatchIngestResponse
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read websocket ack: %v", err)
	}
	if ack.AcceptedCount != 1 || ack.RejectedCount != 0 {
		t.Fatalf("unexpected websocket ack: %+v", ack)
	}
}

func TestHealthKnownPSPNoDataContract(t *testing.T) {
	router := newTestRouter()
	now := time.Now().UTC()

	// Ingest old-but-valid event (inside max age 180m, outside health 60m window).
	ingestBody, _ := json.Marshal(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "old-within-max-age",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 220,
				Timestamp:      now.Add(-120 * time.Minute),
			},
		},
	})
	ingestReq := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(ingestBody))
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("expected ingest 200, got %d", ingestRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/health?psp=PSP_ALPHA", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp model.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(resp.PSPs) != 1 {
		t.Fatalf("expected exactly one PSP entry, got %d", len(resp.PSPs))
	}
	view := resp.PSPs[0]
	if !view.NoData {
		t.Fatalf("expected no_data=true")
	}
	if view.HealthScore != nil {
		t.Fatalf("expected health_score to be null")
	}
	if len(view.Windows) != 0 {
		t.Fatalf("expected empty windows object, got %+v", view.Windows)
	}
}

func TestHealthKnownPSPNoRecent5mScoreIsNull(t *testing.T) {
	router := newTestRouter()
	now := time.Now().UTC()

	// Ingest event inside 60m, but outside 5m and 15m windows.
	ingestBody, _ := json.Marshal(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "inside-60m-outside-15m",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 220,
				Timestamp:      now.Add(-30 * time.Minute),
			},
		},
	})
	ingestReq := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(ingestBody))
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("expected ingest 200, got %d", ingestRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/health?psp=PSP_ALPHA", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp model.HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(resp.PSPs) != 1 {
		t.Fatalf("expected exactly one PSP entry, got %d", len(resp.PSPs))
	}
	view := resp.PSPs[0]
	if view.NoData {
		t.Fatalf("expected no_data=false when 60m has events")
	}
	if view.HealthScore != nil {
		t.Fatalf("expected health_score to be null when 5m window has no events")
	}
	if _, has5m := view.Windows["5m"]; has5m {
		t.Fatalf("expected 5m window to be omitted when aggregate is ok=false")
	}
	if _, has15m := view.Windows["15m"]; has15m {
		t.Fatalf("expected 15m window to be omitted when aggregate is ok=false")
	}
	if _, has60m := view.Windows["60m"]; !has60m {
		t.Fatalf("expected 60m window to be present")
	}
}

func TestEndToEndIngestHealthAlertsComparisonFlow(t *testing.T) {
	router := newTestRouter()
	now := time.Now().UTC().Truncate(time.Minute)

	events := make([]model.TransactionEvent, 0, 16)
	// Create 8 degraded minutes for PSP_BETA to ensure sustained alert.
	for i := 0; i < 8; i++ {
		events = append(events, model.TransactionEvent{
			TransactionID:  "e2e-beta-bad-" + time.Duration(i).String(),
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1800,
			Timestamp:      now.Add(-time.Duration(8-i) * time.Minute),
		})
	}
	// Healthy PSP_ALPHA samples.
	for i := 0; i < 8; i++ {
		events = append(events, model.TransactionEvent{
			TransactionID:  "e2e-alpha-good-" + time.Duration(i).String(),
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 230,
			Timestamp:      now.Add(-time.Duration(8-i) * time.Minute),
		})
	}

	ingestBody, _ := json.Marshal(model.BatchIngestRequest{Events: events})
	ingestReq := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(ingestBody))
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)
	if ingestRec.Code != http.StatusOK {
		t.Fatalf("expected ingest 200, got %d", ingestRec.Code)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	router.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d", healthRec.Code)
	}
	var healthResp model.HealthResponse
	if err := json.Unmarshal(healthRec.Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(healthResp.PSPs) == 0 {
		t.Fatalf("expected health response to contain psps")
	}

	alertsReq := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	alertsRec := httptest.NewRecorder()
	router.ServeHTTP(alertsRec, alertsReq)
	if alertsRec.Code != http.StatusOK {
		t.Fatalf("expected alerts 200, got %d", alertsRec.Code)
	}
	var alertsResp model.AlertsResponse
	if err := json.Unmarshal(alertsRec.Body.Bytes(), &alertsResp); err != nil {
		t.Fatalf("decode alerts response: %v", err)
	}
	if len(alertsResp.Events) == 0 {
		t.Fatalf("expected at least one alert incident")
	}

	compReq := httptest.NewRequest(http.MethodGet, "/comparison", nil)
	compRec := httptest.NewRecorder()
	router.ServeHTTP(compRec, compReq)
	if compRec.Code != http.StatusOK {
		t.Fatalf("expected comparison 200, got %d", compRec.Code)
	}
	var compResp model.ComparisonResponse
	if err := json.Unmarshal(compRec.Body.Bytes(), &compResp); err != nil {
		t.Fatalf("decode comparison response: %v", err)
	}
	if len(compResp.Ranking) == 0 {
		t.Fatalf("expected at least one ranking row")
	}
}
