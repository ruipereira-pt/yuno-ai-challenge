package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func TestAPIIntegrationMiniBatchFlow(t *testing.T) {
	router := newTestRouter()
	now := time.Now().UTC().Truncate(time.Minute)

	events := []model.TransactionEvent{
		// Healthy PSP_ALPHA events.
		{
			TransactionID:  "int-alpha-1",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 220,
			Timestamp:      now.Add(-4 * time.Minute),
		},
		{
			TransactionID:  "int-alpha-2",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 240,
			Timestamp:      now.Add(-3 * time.Minute),
		},
		// Degraded PSP_BETA events in last 5m.
		{
			TransactionID:  "int-beta-1",
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1800,
			Timestamp:      now.Add(-5 * time.Minute),
		},
		{
			TransactionID:  "int-beta-2",
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1700,
			Timestamp:      now.Add(-4 * time.Minute),
		},
		{
			TransactionID:  "int-beta-3",
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1650,
			Timestamp:      now.Add(-3 * time.Minute),
		},
		{
			TransactionID:  "int-beta-4",
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1750,
			Timestamp:      now.Add(-2 * time.Minute),
		},
		{
			TransactionID:  "int-beta-5",
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1850,
			Timestamp:      now.Add(-1 * time.Minute),
		},
	}

	raw, _ := json.Marshal(model.BatchIngestRequest{Events: events})
	ingestReq := httptest.NewRequest(http.MethodPost, "/events/batch", bytes.NewReader(raw))
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestRec := httptest.NewRecorder()
	router.ServeHTTP(ingestRec, ingestReq)

	if ingestRec.Code != http.StatusOK {
		t.Fatalf("expected ingest status 200, got %d", ingestRec.Code)
	}

	var ingestResp model.BatchIngestResponse
	if err := json.Unmarshal(ingestRec.Body.Bytes(), &ingestResp); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	if ingestResp.AcceptedCount != len(events) || ingestResp.DuplicateCount != 0 || ingestResp.RejectedCount != 0 {
		t.Fatalf("unexpected ingest counters: %+v", ingestResp)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	router.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", healthRec.Code)
	}

	var healthResp model.HealthResponse
	if err := json.Unmarshal(healthRec.Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if len(healthResp.PSPs) != 2 {
		t.Fatalf("expected exactly 2 psps, got %d", len(healthResp.PSPs))
	}
	if healthResp.PSPs[0].PSP != "PSP_ALPHA" || healthResp.PSPs[1].PSP != "PSP_BETA" {
		t.Fatalf("unexpected health ordering: first=%s second=%s", healthResp.PSPs[0].PSP, healthResp.PSPs[1].PSP)
	}
	if healthResp.PSPs[0].HealthScore == nil || healthResp.PSPs[1].HealthScore == nil {
		t.Fatalf("expected non-null health_score for both PSPs")
	}
	if healthResp.PSPs[0].NoData || healthResp.PSPs[1].NoData {
		t.Fatalf("expected no_data=false for both PSPs")
	}
	if !healthResp.PSPs[1].Degraded {
		t.Fatalf("expected PSP_BETA to be degraded")
	}

	alertsReq := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	alertsRec := httptest.NewRecorder()
	router.ServeHTTP(alertsRec, alertsReq)
	if alertsRec.Code != http.StatusOK {
		t.Fatalf("expected alerts status 200, got %d", alertsRec.Code)
	}

	var alertsResp model.AlertsResponse
	if err := json.Unmarshal(alertsRec.Body.Bytes(), &alertsResp); err != nil {
		t.Fatalf("decode alerts response: %v", err)
	}
	if len(alertsResp.Events) != 1 {
		t.Fatalf("expected exactly one alert incident, got %d", len(alertsResp.Events))
	}
	if alertsResp.Events[0].PSP != "PSP_BETA" {
		t.Fatalf("expected alert PSP=PSP_BETA, got %s", alertsResp.Events[0].PSP)
	}
	if alertsResp.Events[0].Reason != "health" {
		t.Fatalf("expected alert reason=health, got %s", alertsResp.Events[0].Reason)
	}
	if alertsResp.Events[0].EndedAt != nil {
		t.Fatalf("expected active alert with ended_at=nil")
	}

	compReq := httptest.NewRequest(http.MethodGet, "/comparison", nil)
	compRec := httptest.NewRecorder()
	router.ServeHTTP(compRec, compReq)
	if compRec.Code != http.StatusOK {
		t.Fatalf("expected comparison status 200, got %d", compRec.Code)
	}

	var compResp model.ComparisonResponse
	if err := json.Unmarshal(compRec.Body.Bytes(), &compResp); err != nil {
		t.Fatalf("decode comparison response: %v", err)
	}
	if len(compResp.Ranking) != 2 {
		t.Fatalf("expected exactly 2 ranking rows, got %d", len(compResp.Ranking))
	}
	if compResp.Ranking[0].PSP != "PSP_ALPHA" || compResp.Ranking[1].PSP != "PSP_BETA" {
		t.Fatalf("unexpected ranking order: first=%s second=%s", compResp.Ranking[0].PSP, compResp.Ranking[1].PSP)
	}
	if compResp.Ranking[0].HealthScore <= compResp.Ranking[1].HealthScore {
		t.Fatalf("expected PSP_ALPHA health score > PSP_BETA health score")
	}
}
