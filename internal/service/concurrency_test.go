package service

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

func TestConcurrentIngestAndRead(t *testing.T) {
	windowStore := store.NewWindowStore()
	scorer := NewScorer()
	alerts := NewAlertEvaluator(AlertConfig{
		HealthThreshold:   60,
		ApprovalThreshold: 0.70,
		ErrorThreshold:    0.15,
		SustainedFor:      5 * time.Minute,
	}, scorer)
	svc := NewHealthService(windowStore, scorer, alerts, 180*time.Minute, 2*time.Minute)

	now := time.Now().UTC()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			events := make([]model.TransactionEvent, 0, 5)
			for j := 0; j < 5; j++ {
				id := fmt.Sprintf("conc-%d-%d", i, j)
				status := model.StatusApproved
				if j%4 == 0 {
					status = model.StatusError
				}
				events = append(events, model.TransactionEvent{
					TransactionID:  id,
					PSP:            "PSP_ALPHA",
					Status:         status,
					ResponseTimeMs: 300 + j,
					Timestamp:      now.Add(-time.Duration((i+j)%30) * time.Minute),
				})
			}
			_ = svc.IngestBatch(model.BatchIngestRequest{Events: events})
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = svc.GetHealth("")
			_ = svc.GetComparison(now.Add(-60*time.Minute), now)
			_ = svc.GetAlerts("", now.Add(-60*time.Minute), now, false)
		}
	}()

	wg.Wait()
}
