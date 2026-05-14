package store

import (
	"testing"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func TestAggregateWindowBoundariesStartInclusiveEndExclusive(t *testing.T) {
	ws := NewWindowStore()
	now := time.Now().UTC().Truncate(time.Minute)
	start := now.Add(-5 * time.Minute)
	end := now

	events := []model.TransactionEvent{
		{
			TransactionID:  "tx-before-start",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 200,
			Timestamp:      start.Add(-1 * time.Second),
		},
		{
			TransactionID:  "tx-at-start",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 210,
			Timestamp:      start,
		},
		{
			TransactionID:  "tx-middle",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusDeclined,
			ResponseTimeMs: 220,
			Timestamp:      start.Add(2 * time.Minute),
		},
		{
			TransactionID:  "tx-at-end",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusError,
			ResponseTimeMs: 230,
			Timestamp:      end,
		},
	}
	accepted, duplicates := ws.IngestBatch(events)
	if accepted != 4 || duplicates != 0 {
		t.Fatalf("unexpected ingest counts accepted=%d duplicates=%d", accepted, duplicates)
	}

	metrics, ok := ws.Aggregate("PSP_ALPHA", start, end)
	if !ok {
		t.Fatalf("expected aggregate data")
	}
	if metrics.Total != 2 {
		t.Fatalf("expected total=2 (start included, end excluded), got %d", metrics.Total)
	}
	if metrics.Approved != 1 || metrics.Declined != 1 || metrics.Error != 0 {
		t.Fatalf("unexpected status counts: %+v", metrics)
	}
}

func TestAggregateWindowBoundariesForAllConfiguredWindows(t *testing.T) {
	ws := NewWindowStore()
	now := time.Now().UTC().Truncate(time.Minute)
	m15 := now.Add(-15 * time.Minute)
	m60 := now.Add(-60 * time.Minute)

	events := []model.TransactionEvent{
		{
			TransactionID:  "tx-at-15",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 250,
			Timestamp:      m15,
		},
		{
			TransactionID:  "tx-at-60",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusApproved,
			ResponseTimeMs: 260,
			Timestamp:      m60,
		},
		{
			TransactionID:  "tx-at-now",
			PSP:            "PSP_ALPHA",
			Status:         model.StatusError,
			ResponseTimeMs: 900,
			Timestamp:      now,
		},
	}
	ws.IngestBatch(events)

	m5, ok5 := ws.Aggregate("PSP_ALPHA", now.Add(-5*time.Minute), now)
	if ok5 || m5.Total != 0 {
		t.Fatalf("expected 5m to have no data (ok=false,total=0), got ok=%v total=%d", ok5, m5.Total)
	}

	m15Agg, ok15 := ws.Aggregate("PSP_ALPHA", m15, now)
	if !ok15 || m15Agg.Total != 1 {
		t.Fatalf("expected 15m total=1, got ok=%v total=%d", ok15, m15Agg.Total)
	}

	m60Agg, ok60 := ws.Aggregate("PSP_ALPHA", m60, now)
	if !ok60 || m60Agg.Total != 2 {
		t.Fatalf("expected 60m total=2, got ok=%v total=%d", ok60, m60Agg.Total)
	}
}
