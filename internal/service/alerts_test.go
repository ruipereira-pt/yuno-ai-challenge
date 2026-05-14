package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

func TestAlertTransitionsTableDriven(t *testing.T) {
	tests := []struct {
		name                string
		buildEvents         func(now time.Time) []model.TransactionEvent
		expectHasIncident   bool
		expectLatestClosed  bool
		expectOpeningReason string
		recomputeAt         func(now time.Time) time.Time
	}{
		{
			name: "opens after sustained degradation",
			buildEvents: func(now time.Time) []model.TransactionEvent {
				out := make([]model.TransactionEvent, 0, 8)
				for i := 0; i < 8; i++ {
					out = append(out, model.TransactionEvent{
						TransactionID:  fmt.Sprintf("open-%d", i),
						PSP:            "PSP_BETA",
						Status:         model.StatusError,
						ResponseTimeMs: 1800,
						Timestamp:      now.Add(-time.Duration(8-i) * time.Minute),
					})
				}
				return out
			},
			expectHasIncident:   true,
			expectLatestClosed:  false,
			expectOpeningReason: "health",
			recomputeAt: func(now time.Time) time.Time {
				return now
			},
		},
		{
			name: "closes after healthy recovery",
			buildEvents: func(now time.Time) []model.TransactionEvent {
				out := make([]model.TransactionEvent, 0, 20)
				// First 10 minutes degraded.
				for i := 0; i < 10; i++ {
					out = append(out, model.TransactionEvent{
						TransactionID:  fmt.Sprintf("close-bad-%d", i),
						PSP:            "PSP_BETA",
						Status:         model.StatusError,
						ResponseTimeMs: 1900,
						Timestamp:      now.Add(-time.Duration(20-i) * time.Minute),
					})
				}
				// Then 10 minutes healthy.
				for i := 0; i < 10; i++ {
					out = append(out, model.TransactionEvent{
						TransactionID:  fmt.Sprintf("close-good-%d", i),
						PSP:            "PSP_BETA",
						Status:         model.StatusApproved,
						ResponseTimeMs: 260,
						Timestamp:      now.Add(-time.Duration(10-i) * time.Minute),
					})
				}
				return out
			},
			expectHasIncident:   true,
			expectLatestClosed:  true,
			expectOpeningReason: "health",
			recomputeAt: func(now time.Time) time.Time {
				return now
			},
		},
		{
			name: "four minute degradation does not open",
			buildEvents: func(now time.Time) []model.TransactionEvent {
				out := make([]model.TransactionEvent, 0, 4)
				// Only 4 degraded minutes and then no data.
				for i := 0; i < 4; i++ {
					out = append(out, model.TransactionEvent{
						TransactionID:  fmt.Sprintf("short-bad-%d", i),
						PSP:            "PSP_BETA",
						Status:         model.StatusError,
						ResponseTimeMs: 1700,
						Timestamp:      now.Add(-time.Duration(4-i) * time.Minute),
					})
				}
				return out
			},
			expectHasIncident:   false,
			expectLatestClosed:  false,
			expectOpeningReason: "",
			recomputeAt: func(now time.Time) time.Time {
				return now
			},
		},
		{
			name: "no data does not auto close active incident",
			buildEvents: func(now time.Time) []model.TransactionEvent {
				out := make([]model.TransactionEvent, 0, 8)
				for i := 0; i < 8; i++ {
					out = append(out, model.TransactionEvent{
						TransactionID:  fmt.Sprintf("nodata-bad-%d", i),
						PSP:            "PSP_BETA",
						Status:         model.StatusError,
						ResponseTimeMs: 1800,
						Timestamp:      now.Add(-time.Duration(20-i) * time.Minute),
					})
				}
				return out
			},
			expectHasIncident:   true,
			expectLatestClosed:  false,
			expectOpeningReason: "health",
			recomputeAt: func(now time.Time) time.Time {
				// advance evaluation horizon without adding new events
				return now.Add(15 * time.Minute)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			windowStore := store.NewWindowStore()
			scorer := NewScorer()
			alerts := NewAlertEvaluator(AlertConfig{
				HealthThreshold:   60,
				ApprovalThreshold: 0.70,
				ErrorThreshold:    0.15,
				SustainedFor:      5 * time.Minute,
			}, scorer)

			now := time.Now().UTC().Truncate(time.Minute)
			windowStore.IngestBatch(tc.buildEvents(now))
			recomputeAt := tc.recomputeAt(now)
			alerts.Recompute(windowStore, recomputeAt, 180*time.Minute, 1)

			resp := windowStore.GetAlerts("PSP_BETA", recomputeAt.Add(-120*time.Minute), recomputeAt.Add(1*time.Minute), false)
			if tc.expectHasIncident && len(resp.Events) == 0 {
				t.Fatalf("expected at least one alert incident")
			}
			if !tc.expectHasIncident && len(resp.Events) != 0 {
				t.Fatalf("expected no incidents, got %d", len(resp.Events))
			}
			if len(resp.Events) == 0 {
				return
			}
			if resp.Events[0].Reason != tc.expectOpeningReason {
				t.Fatalf("expected opening reason %q, got %q", tc.expectOpeningReason, resp.Events[0].Reason)
			}

			latest := resp.Events[len(resp.Events)-1]
			if tc.expectLatestClosed && latest.EndedAt == nil {
				t.Fatalf("expected latest incident to be closed")
			}
			if !tc.expectLatestClosed && latest.EndedAt != nil {
				t.Fatalf("expected latest incident to remain active")
			}
		})
	}
}

func TestAlertSingleActiveIncidentAndReasonLocked(t *testing.T) {
	windowStore := store.NewWindowStore()
	scorer := NewScorer()
	alerts := NewAlertEvaluator(AlertConfig{
		HealthThreshold:   60,
		ApprovalThreshold: 0.70,
		ErrorThreshold:    0.15,
		SustainedFor:      5 * time.Minute,
	}, scorer)

	now := time.Now().UTC().Truncate(time.Minute)

	// Phase 1: 6 minutes of severe degradation (health reason should open).
	phaseOne := make([]model.TransactionEvent, 0, 6)
	for i := 0; i < 6; i++ {
		phaseOne = append(phaseOne, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("lock-phase1-%d", i),
			PSP:            "PSP_BETA",
			Status:         model.StatusError,
			ResponseTimeMs: 1900,
			Timestamp:      now.Add(-time.Duration(12-i) * time.Minute),
		})
	}
	windowStore.IngestBatch(phaseOne)
	alerts.Recompute(windowStore, now.Add(-6*time.Minute), 180*time.Minute, 1)

	// Phase 2: still degraded but now mostly approval-rate based.
	phaseTwo := make([]model.TransactionEvent, 0, 6)
	for i := 0; i < 6; i++ {
		status := model.StatusApproved
		if i%2 == 0 {
			status = model.StatusDeclined
		}
		phaseTwo = append(phaseTwo, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("lock-phase2-%d", i),
			PSP:            "PSP_BETA",
			Status:         status,
			ResponseTimeMs: 260,
			Timestamp:      now.Add(-time.Duration(6-i) * time.Minute),
		})
	}
	windowStore.IngestBatch(phaseTwo)
	alerts.Recompute(windowStore, now, 180*time.Minute, 2)

	all := windowStore.GetAlerts("PSP_BETA", now.Add(-180*time.Minute), now.Add(1*time.Minute), false)
	active := windowStore.GetAlerts("PSP_BETA", now.Add(-180*time.Minute), now.Add(1*time.Minute), true)

	if len(all.Events) == 0 {
		t.Fatalf("expected at least one incident")
	}
	if all.Events[0].Reason != "health" {
		t.Fatalf("expected opening reason to remain 'health', got %q", all.Events[0].Reason)
	}
	if len(active.Events) > 1 {
		t.Fatalf("expected at most one active incident, got %d", len(active.Events))
	}
}
