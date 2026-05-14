package service

import (
	"sort"
	"strings"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

type HealthService struct {
	store         *store.WindowStore
	scorer        *Scorer
	alerts        *AlertEvaluator
	maxEventAge   time.Duration
	maxFutureSkew time.Duration
}

func NewHealthService(
	windowStore *store.WindowStore,
	scorer *Scorer,
	alerts *AlertEvaluator,
	maxEventAge time.Duration,
	maxFutureSkew time.Duration,
) *HealthService {
	return &HealthService{
		store:         windowStore,
		scorer:        scorer,
		alerts:        alerts,
		maxEventAge:   maxEventAge,
		maxFutureSkew: maxFutureSkew,
	}
}

func (s *HealthService) IngestBatch(req model.BatchIngestRequest) model.BatchIngestResponse {
	now := time.Now().UTC()
	normalized := make([]model.TransactionEvent, 0, len(req.Events))

	for _, evt := range req.Events {
		normalized = append(normalized, normalizeEvent(evt))
	}

	resp, version := s.store.ClassifyAndIngest(normalized, func(evt model.TransactionEvent) (model.BatchIngestError, bool) {
		return s.validateEvent(evt, now)
	})
	s.store.PruneEventsBefore(now.Add(-s.maxEventAge))
	s.alerts.Recompute(s.store, now, s.maxEventAge, version)

	return resp
}

func (s *HealthService) GetHealth(psp string) model.HealthResponse {
	now := time.Now().UTC()
	psps := s.store.PSPs()
	if psp != "" {
		matched := false
		for _, candidate := range psps {
			if candidate == psp {
				matched = true
				break
			}
		}
		if !matched {
			return model.HealthResponse{GeneratedAt: now, PSPs: []model.PSPHealth{}}
		}
		psps = []string{psp}
	}

	out := make([]model.PSPHealth, 0, len(psps))
	for _, candidate := range psps {
		view := model.PSPHealth{
			PSP:     candidate,
			Windows: map[string]model.WindowMetrics{},
		}

		metrics60, has60 := s.store.Aggregate(candidate, now.Add(-60*time.Minute), now)
		if !has60 {
			view.NoData = true
			view.Windows = map[string]model.WindowMetrics{}
			view.HealthScore = nil
			view.Degraded = false
			out = append(out, view)
			continue
		}

		metrics5, has5 := s.store.Aggregate(candidate, now.Add(-5*time.Minute), now)
		metrics15, has15 := s.store.Aggregate(candidate, now.Add(-15*time.Minute), now)
		if has5 {
			view.Windows["5m"] = metrics5
		}
		if has15 {
			view.Windows["15m"] = metrics15
		}
		view.Windows["60m"] = metrics60
		if has5 {
			score := s.scorer.Score(metrics5)
			view.HealthScore = &score
		} else {
			// A zero-event 5m aggregate is unknown (ok=false), not a real score.
			view.HealthScore = nil
		}
		view.NoData = false
		view.Degraded = s.store.IsPSPDegraded(candidate)
		out = append(out, view)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].PSP < out[j].PSP
	})
	return model.HealthResponse{
		GeneratedAt: now,
		PSPs:        out,
	}
}

func (s *HealthService) GetAlerts(psp string, from time.Time, to time.Time, activeOnly bool) model.AlertsResponse {
	return s.store.GetAlerts(psp, from, to, activeOnly)
}

func (s *HealthService) GetComparison(from time.Time, to time.Time) model.ComparisonResponse {
	resp := model.ComparisonResponse{
		Ranking: []model.ComparisonRow{},
	}
	resp.Range.From = from
	resp.Range.To = to

	for _, psp := range s.store.PSPs() {
		metrics, ok := s.store.Aggregate(psp, from, to)
		if !ok {
			continue
		}
		resp.Ranking = append(resp.Ranking, model.ComparisonRow{
			PSP:               psp,
			HealthScore:       s.scorer.Score(metrics),
			ApprovalRate:      metrics.ApprovalRate,
			AvgResponseTimeMs: metrics.AvgResponseTimeMs,
		})
	}
	sort.Slice(resp.Ranking, func(i, j int) bool {
		left := resp.Ranking[i]
		right := resp.Ranking[j]
		if left.HealthScore != right.HealthScore {
			return left.HealthScore > right.HealthScore
		}
		if left.ApprovalRate != right.ApprovalRate {
			return left.ApprovalRate > right.ApprovalRate
		}
		if left.AvgResponseTimeMs != right.AvgResponseTimeMs {
			return left.AvgResponseTimeMs < right.AvgResponseTimeMs
		}
		return left.PSP < right.PSP
	})
	return resp
}

func normalizeEvent(evt model.TransactionEvent) model.TransactionEvent {
	evt.PSP = strings.TrimSpace(evt.PSP)
	evt.Status = strings.ToLower(strings.TrimSpace(evt.Status))
	evt.TransactionID = strings.TrimSpace(evt.TransactionID)
	evt.Timestamp = evt.Timestamp.UTC()
	return evt
}

func (s *HealthService) validateEvent(evt model.TransactionEvent, now time.Time) (model.BatchIngestError, bool) {
	if evt.TransactionID == "" {
		return ingestError(evt, "missing_transaction_id", "transaction_id is required"), true
	}
	if evt.PSP == "" {
		return ingestError(evt, "missing_psp", "psp is required"), true
	}
	if evt.Status != model.StatusApproved && evt.Status != model.StatusDeclined && evt.Status != model.StatusError {
		return ingestError(evt, "invalid_status", "status must be approved, declined, or error"), true
	}
	if evt.Timestamp.IsZero() {
		return ingestError(evt, "invalid_timestamp", "timestamp is required"), true
	}
	if evt.Timestamp.UTC().Before(now.Add(-s.maxEventAge)) {
		return ingestError(evt, "event_too_old", "event timestamp is older than maximum age"), true
	}
	if evt.Timestamp.UTC().After(now.Add(s.maxFutureSkew)) {
		return ingestError(evt, "event_in_future", "event timestamp exceeds allowed future skew"), true
	}
	if evt.ResponseTimeMs < 0 {
		return ingestError(evt, "invalid_response_time", "response_time_ms must be >= 0"), true
	}
	return model.BatchIngestError{}, false
}

func ingestError(evt model.TransactionEvent, code string, message string) model.BatchIngestError {
	return model.BatchIngestError{
		TransactionID: evt.TransactionID,
		Code:          code,
		Message:       message,
	}
}
