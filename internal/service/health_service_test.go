package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

func newTestService() *HealthService {
	windowStore := store.NewWindowStore()
	scorer := NewScorer()
	alerts := NewAlertEvaluator(AlertConfig{
		HealthThreshold:   60,
		ApprovalThreshold: 0.70,
		ErrorThreshold:    0.15,
		SustainedFor:      5 * time.Minute,
	}, scorer)
	return NewHealthService(windowStore, scorer, alerts, 180*time.Minute, 2*time.Minute)
}

func TestIngestBatchDuplicatePrecedence(t *testing.T) {
	svc := newTestService()
	now := time.Now().UTC()

	first := svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-1",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 250,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	})
	if first.AcceptedCount != 1 {
		t.Fatalf("expected first accepted_count=1, got %d", first.AcceptedCount)
	}

	second := svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-1",
				PSP:            "",
				Status:         "bad",
				ResponseTimeMs: -1,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	})

	if second.DuplicateCount != 1 {
		t.Fatalf("expected duplicate_count=1, got %d", second.DuplicateCount)
	}
	if second.RejectedCount != 0 {
		t.Fatalf("expected rejected_count=0 for duplicate-first behavior, got %d", second.RejectedCount)
	}
}

func TestIngestBatchTimeValidation(t *testing.T) {
	svc := newTestService()
	now := time.Now().UTC()

	tests := []struct {
		name         string
		event        model.TransactionEvent
		expectedCode string
	}{
		{
			name: "missing transaction id",
			event: model.TransactionEvent{
				TransactionID:  "",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 200,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			expectedCode: "missing_transaction_id",
		},
		{
			name: "missing psp",
			event: model.TransactionEvent{
				TransactionID:  "tx-missing-psp",
				PSP:            "",
				Status:         model.StatusApproved,
				ResponseTimeMs: 200,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			expectedCode: "missing_psp",
		},
		{
			name: "invalid status",
			event: model.TransactionEvent{
				TransactionID:  "tx-invalid-status",
				PSP:            "PSP_ALPHA",
				Status:         "pending",
				ResponseTimeMs: 200,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			expectedCode: "invalid_status",
		},
		{
			name: "invalid timestamp",
			event: model.TransactionEvent{
				TransactionID:  "tx-invalid-timestamp",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 200,
				Timestamp:      time.Time{},
			},
			expectedCode: "invalid_timestamp",
		},
		{
			name: "event too old",
			event: model.TransactionEvent{
				TransactionID:  "tx-too-old",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 200,
				Timestamp:      now.Add(-181 * time.Minute),
			},
			expectedCode: "event_too_old",
		},
		{
			name: "event in future",
			event: model.TransactionEvent{
				TransactionID:  "tx-future",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 200,
				Timestamp:      now.Add(3 * time.Minute),
			},
			expectedCode: "event_in_future",
		},
		{
			name: "invalid response time",
			event: model.TransactionEvent{
				TransactionID:  "tx-negative-latency",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: -1,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			expectedCode: "invalid_response_time",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := svc.IngestBatch(model.BatchIngestRequest{
				Events: []model.TransactionEvent{tc.event},
			})

			if resp.RejectedCount != 1 {
				t.Fatalf("expected rejected_count=1, got %d", resp.RejectedCount)
			}
			if len(resp.Errors) != 1 {
				t.Fatalf("expected one ingest error, got %d", len(resp.Errors))
			}
			if resp.Errors[0].Code != tc.expectedCode {
				t.Fatalf("expected error code %s, got %s", tc.expectedCode, resp.Errors[0].Code)
			}
		})
	}
}

func TestComparisonSorting(t *testing.T) {
	svc := newTestService()
	now := time.Now().UTC()

	_ = svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "a-1",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 220,
				Timestamp:      now.Add(-2 * time.Minute),
			},
			{
				TransactionID:  "a-2",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 240,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "b-1",
				PSP:            "PSP_BETA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 600,
				Timestamp:      now.Add(-2 * time.Minute),
			},
			{
				TransactionID:  "b-2",
				PSP:            "PSP_BETA",
				Status:         model.StatusError,
				ResponseTimeMs: 1200,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	})

	resp := svc.GetComparison(now.Add(-10*time.Minute), now)
	if len(resp.Ranking) != 2 {
		t.Fatalf("expected 2 ranking rows, got %d", len(resp.Ranking))
	}
	if resp.Ranking[0].PSP != "PSP_ALPHA" {
		t.Fatalf("expected PSP_ALPHA first, got %s", resp.Ranking[0].PSP)
	}
}

func TestIngestBatchDuplicateWithinSameBatch(t *testing.T) {
	svc := newTestService()
	now := time.Now().UTC()

	resp := svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-batch-dup",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 210,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-batch-dup",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 211,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	})

	if resp.AcceptedCount != 1 || resp.DuplicateCount != 1 || resp.RejectedCount != 0 {
		t.Fatalf("unexpected ingest result: %+v", resp)
	}
}

func TestIngestBatchMixedCountsAtomicContract(t *testing.T) {
	svc := newTestService()
	now := time.Now().UTC()

	// Seed history duplicate.
	seed := svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-history-dup",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 210,
				Timestamp:      now.Add(-2 * time.Minute),
			},
		},
	})
	if seed.AcceptedCount != 1 {
		t.Fatalf("expected seed accepted_count=1, got %d", seed.AcceptedCount)
	}

	resp := svc.IngestBatch(model.BatchIngestRequest{
		Events: []model.TransactionEvent{
			{
				TransactionID:  "tx-valid-1",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 220,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-valid-1", // duplicate in same batch
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 221,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-history-dup", // duplicate across history
				PSP:            "PSP_BETA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 222,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-invalid-psp",
				PSP:            "",
				Status:         model.StatusApproved,
				ResponseTimeMs: 230,
				Timestamp:      now.Add(-1 * time.Minute),
			},
			{
				TransactionID:  "tx-invalid-future",
				PSP:            "PSP_ALPHA",
				Status:         model.StatusApproved,
				ResponseTimeMs: 230,
				Timestamp:      now.Add(3 * time.Minute),
			},
			{
				TransactionID:  "tx-valid-2",
				PSP:            "PSP_BETA",
				Status:         model.StatusDeclined,
				ResponseTimeMs: 240,
				Timestamp:      now.Add(-1 * time.Minute),
			},
		},
	})

	if resp.AcceptedCount != 2 {
		t.Fatalf("expected accepted_count=2, got %d", resp.AcceptedCount)
	}
	if resp.DuplicateCount != 2 {
		t.Fatalf("expected duplicate_count=2, got %d", resp.DuplicateCount)
	}
	if resp.RejectedCount != 2 {
		t.Fatalf("expected rejected_count=2, got %d", resp.RejectedCount)
	}
	if len(resp.Errors) != 2 {
		t.Fatalf("expected two error entries, got %d", len(resp.Errors))
	}
}

func TestComparisonSortingTieBreakers(t *testing.T) {
	tests := []struct {
		name      string
		events    []model.TransactionEvent
		expected0 string
		expected1 string
	}{
		{
			name: "same health different approval then approval desc",
			events: append(
				buildEvents("PSP_A", 10, 0, 0, 1320, time.Now().UTC()),
				buildEvents("PSP_B", 9, 1, 0, 200, time.Now().UTC())...,
			),
			expected0: "PSP_A",
			expected1: "PSP_B",
		},
		{
			name: "same health same approval different latency then latency asc",
			events: append(
				buildEvents("PSP_C", 9, 1, 0, 1320, time.Now().UTC()),
				buildEvents("PSP_D", 9, 0, 1, 853, time.Now().UTC())...,
			),
			expected0: "PSP_D",
			expected1: "PSP_C",
		},
		{
			name: "same health approval latency then psp lexical asc",
			events: append(
				buildEvents("PSP_X", 9, 1, 0, 500, time.Now().UTC()),
				buildEvents("PSP_Y", 9, 1, 0, 500, time.Now().UTC())...,
			),
			expected0: "PSP_X",
			expected1: "PSP_Y",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			now := time.Now().UTC()
			_ = svc.IngestBatch(model.BatchIngestRequest{Events: tc.events})

			resp := svc.GetComparison(now.Add(-30*time.Minute), now.Add(1*time.Minute))
			if len(resp.Ranking) < 2 {
				t.Fatalf("expected at least 2 ranking rows, got %d", len(resp.Ranking))
			}
			if resp.Ranking[0].PSP != tc.expected0 || resp.Ranking[1].PSP != tc.expected1 {
				t.Fatalf("unexpected ranking order: first=%s second=%s", resp.Ranking[0].PSP, resp.Ranking[1].PSP)
			}
		})
	}
}

func buildEvents(psp string, approved int, declined int, errors int, latency int, now time.Time) []model.TransactionEvent {
	out := make([]model.TransactionEvent, 0, approved+declined+errors)
	idx := 0
	for i := 0; i < approved; i++ {
		out = append(out, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("%s-ap-%d", psp, idx),
			PSP:            psp,
			Status:         model.StatusApproved,
			ResponseTimeMs: latency,
			Timestamp:      now.Add(-2 * time.Minute),
		})
		idx++
	}
	for i := 0; i < declined; i++ {
		out = append(out, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("%s-de-%d", psp, idx),
			PSP:            psp,
			Status:         model.StatusDeclined,
			ResponseTimeMs: latency,
			Timestamp:      now.Add(-2 * time.Minute),
		})
		idx++
	}
	for i := 0; i < errors; i++ {
		out = append(out, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("%s-er-%d", psp, idx),
			PSP:            psp,
			Status:         model.StatusError,
			ResponseTimeMs: latency,
			Timestamp:      now.Add(-2 * time.Minute),
		})
		idx++
	}
	return out
}

func BenchmarkIngestBatch(b *testing.B) {
	svc := newTestService()
	now := time.Now().UTC()
	events := make([]model.TransactionEvent, 1000)
	for i := 0; i < len(events); i++ {
		events[i] = model.TransactionEvent{
			TransactionID:  fmt.Sprintf("bench-%d", i),
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 250,
			Timestamp:      now.Add(-time.Duration(i%60) * time.Minute),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.IngestBatch(model.BatchIngestRequest{Events: events})
	}
}
