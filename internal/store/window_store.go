package store

import (
	"sort"
	"sync"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

type PSPState struct {
	Events []model.TransactionEvent
}

type WindowStore struct {
	mu              sync.RWMutex
	byPSP           map[string]*PSPState
	order           []string
	seenEventIDs    map[string]time.Time
	alertsByPSP     map[string][]model.AlertIncident
	activeByPSP     map[string]*model.AlertIncident
	ingestVersion   uint64
	alertVersionPSP map[string]uint64
}

func NewWindowStore() *WindowStore {
	return &WindowStore{
		byPSP:           make(map[string]*PSPState),
		order:           make([]string, 0),
		seenEventIDs:    make(map[string]time.Time),
		alertsByPSP:     make(map[string][]model.AlertIncident),
		activeByPSP:     make(map[string]*model.AlertIncident),
		alertVersionPSP: make(map[string]uint64),
	}
}

func (s *WindowStore) IngestBatch(events []model.TransactionEvent) (accepted int, duplicates int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, evt := range events {
		if _, exists := s.seenEventIDs[evt.TransactionID]; exists {
			duplicates++
			continue
		}
		state, ok := s.byPSP[evt.PSP]
		if !ok {
			state = &PSPState{Events: make([]model.TransactionEvent, 0, 256)}
			s.byPSP[evt.PSP] = state
			s.order = append(s.order, evt.PSP)
		}
		state.Events = append(state.Events, evt)
		s.seenEventIDs[evt.TransactionID] = evt.Timestamp.UTC()
		accepted++
	}

	return accepted, duplicates
}

func (s *WindowStore) ClassifyAndIngest(
	events []model.TransactionEvent,
	validate func(model.TransactionEvent) (model.BatchIngestError, bool),
) (model.BatchIngestResponse, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	response := model.BatchIngestResponse{
		Errors: make([]model.BatchIngestError, 0),
	}
	pending := make([]model.TransactionEvent, 0, len(events))
	seenInBatch := make(map[string]struct{}, len(events))

	for idx, evt := range events {
		txID := evt.TransactionID
		if txID != "" {
			if _, exists := s.seenEventIDs[txID]; exists {
				response.DuplicateCount++
				continue
			}
			if _, exists := seenInBatch[txID]; exists {
				response.DuplicateCount++
				continue
			}
		}

		if reject, rejected := validate(evt); rejected {
			reject.Index = idx
			response.Errors = append(response.Errors, reject)
			continue
		}

		pending = append(pending, evt)
		if txID != "" {
			seenInBatch[txID] = struct{}{}
		}
	}

	for _, evt := range pending {
		state, ok := s.byPSP[evt.PSP]
		if !ok {
			state = &PSPState{Events: make([]model.TransactionEvent, 0, 256)}
			s.byPSP[evt.PSP] = state
			s.order = append(s.order, evt.PSP)
		}
		state.Events = append(state.Events, evt)
		if evt.TransactionID != "" {
			s.seenEventIDs[evt.TransactionID] = evt.Timestamp.UTC()
		}
	}

	response.AcceptedCount = len(pending)
	response.RejectedCount = len(response.Errors)
	s.ingestVersion++
	return response, s.ingestVersion
}

func (s *WindowStore) PruneEventsBefore(cutoff time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffTime := cutoff.UTC()
	for _, state := range s.byPSP {
		next := state.Events[:0]
		for _, evt := range state.Events {
			if !evt.Timestamp.UTC().Before(cutoffTime) {
				next = append(next, evt)
			}
		}
		state.Events = next
	}

	// Keep dedupe bounded to retained event horizon.
	// IDs older than current event retention are evicted.
	retainedIDs := make(map[string]time.Time)
	for _, state := range s.byPSP {
		for _, evt := range state.Events {
			if evt.TransactionID == "" {
				continue
			}
			ts := evt.Timestamp.UTC()
			if prev, ok := retainedIDs[evt.TransactionID]; !ok || ts.After(prev) {
				retainedIDs[evt.TransactionID] = ts
			}
		}
	}
	s.seenEventIDs = retainedIDs
}

func (s *WindowStore) SnapshotBuckets(pspFilter string) map[string][]MinuteBucketView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string][]MinuteBucketView)
	appendState := func(psp string, state *PSPState) {
		aggregated := make(map[int64]*minuteBucket)
		for _, evt := range state.Events {
			minuteStart := evt.Timestamp.UTC().Truncate(time.Minute)
			key := minuteStart.Unix()
			bucket, ok := aggregated[key]
			if !ok {
				bucket = &minuteBucket{MinuteStart: minuteStart}
				aggregated[key] = bucket
			}
			bucket.add(evt.Status, evt.ResponseTimeMs)
		}
		buckets := make([]MinuteBucketView, 0, len(aggregated))
		for _, bucket := range aggregated {
			buckets = append(buckets, MinuteBucketView{
				MinuteStart: bucket.MinuteStart,
				Total:       bucket.Total,
				Approved:    bucket.Approved,
				Declined:    bucket.Declined,
				Error:       bucket.Error,
				LatencySum:  bucket.LatencySum,
			})
		}
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].MinuteStart.Before(buckets[j].MinuteStart)
		})
		out[psp] = buckets
	}

	if pspFilter != "" {
		state, ok := s.byPSP[pspFilter]
		if !ok {
			return out
		}
		appendState(pspFilter, state)
		return out
	}

	for psp, state := range s.byPSP {
		appendState(psp, state)
	}
	return out
}

func (s *WindowStore) SetIncidents(psp string, incidents []model.AlertIncident, active *model.AlertIncident, version uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if version < s.alertVersionPSP[psp] {
		return
	}
	s.alertVersionPSP[psp] = version

	copied := make([]model.AlertIncident, len(incidents))
	copy(copied, incidents)
	s.alertsByPSP[psp] = copied
	if active == nil {
		delete(s.activeByPSP, psp)
		return
	}
	activeCopy := *active
	s.activeByPSP[psp] = &activeCopy
}

func (s *WindowStore) GetAlerts(psp string, from time.Time, to time.Time, activeOnly bool) model.AlertsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := model.AlertsResponse{PSP: psp, Events: []model.AlertIncident{}}
	psps := make([]string, 0)
	if psp != "" {
		psps = append(psps, psp)
	} else {
		psps = append(psps, s.order...)
	}

	for _, candidate := range psps {
		incidents := s.alertsByPSP[candidate]
		for _, incident := range incidents {
			if activeOnly && incident.EndedAt != nil {
				continue
			}
			if !incident.StartedAt.Before(to) {
				continue
			}
			if incident.EndedAt != nil && !incident.EndedAt.After(from) {
				continue
			}
			resp.Events = append(resp.Events, incident)
		}
	}
	sort.Slice(resp.Events, func(i, j int) bool {
		left := resp.Events[i]
		right := resp.Events[j]
		if left.StartedAt.Equal(right.StartedAt) {
			return left.PSP < right.PSP
		}
		return left.StartedAt.After(right.StartedAt)
	})
	return resp
}

func (s *WindowStore) IsPSPDegraded(psp string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeByPSP[psp] != nil
}

func (s *WindowStore) Aggregate(psp string, start time.Time, end time.Time) (model.WindowMetrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.byPSP[psp]
	if !ok {
		return model.WindowMetrics{}, false
	}

	var metrics model.WindowMetrics
	startUTC := start.UTC()
	endUTC := end.UTC()
	for _, evt := range state.Events {
		ts := evt.Timestamp.UTC()
		if ts.Before(startUTC) || !ts.Before(endUTC) {
			continue
		}
		metrics.Total++
		metrics.AvgResponseTimeMs += float64(evt.ResponseTimeMs)
		switch evt.Status {
		case model.StatusApproved:
			metrics.Approved++
		case model.StatusDeclined:
			metrics.Declined++
		case model.StatusError:
			metrics.Error++
		}
	}
	if metrics.Total == 0 {
		return model.WindowMetrics{}, false
	}
	metrics.ApprovalRate = float64(metrics.Approved) / float64(metrics.Total)
	metrics.ErrorRate = float64(metrics.Error) / float64(metrics.Total)
	metrics.AvgResponseTimeMs = metrics.AvgResponseTimeMs / float64(metrics.Total)
	return metrics, true
}

func (s *WindowStore) PSPs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}
