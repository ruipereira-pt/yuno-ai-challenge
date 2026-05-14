package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func (h *Handler) GetEventsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, errorPayload("method_not_allowed", "method not allowed", nil))
		return
	}
	if !h.stream.Enabled {
		writeJSON(w, http.StatusNotFound, errorPayload("not_found", "stream ingest endpoint is disabled", nil))
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadLimit(h.stream.MaxFrameBytes)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	eventCh := make(chan model.TransactionEvent, h.stream.QueueSize)
	ackCh := make(chan model.BatchIngestResponse, 64)

	var wg sync.WaitGroup

	// Single writer goroutine for websocket responses.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case ack, ok := <-ackCh:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(h.stream.ReadTimeout))
				if err := conn.WriteJSON(ack); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Micro-batch ingestor to avoid per-message recompute overhead.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ackCh)

		ticker := time.NewTicker(h.stream.FlushInterval)
		defer ticker.Stop()

		batch := make([]model.TransactionEvent, 0, h.stream.BatchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			resp := h.health.IngestBatch(model.BatchIngestRequest{Events: batch})
			if !enqueueAck(ctx, ackCh, resp, h.stream.ReadTimeout) {
				cancel()
				return
			}
			batch = batch[:0]
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case evt, ok := <-eventCh:
				if !ok {
					flush()
					return
				}
				batch = append(batch, evt)
				if len(batch) >= h.stream.BatchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()

	for {
		_ = conn.SetReadDeadline(time.Now().Add(h.stream.ReadTimeout))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}

		events, decodeErr := decodeStreamPayload(payload)
		if decodeErr != nil {
			if !enqueueAck(ctx, ackCh, model.BatchIngestResponse{
				AcceptedCount:  0,
				DuplicateCount: 0,
				RejectedCount:  1,
				Errors: []model.BatchIngestError{
					{
						Index:         0,
						TransactionID: "",
						Code:          "invalid_request",
						Message:       "payload must be a transaction event or {\"events\": [...]}",
					},
				},
			}, h.stream.ReadTimeout) {
				cancel()
				break
			}
			continue
		}

		overflow := make([]model.BatchIngestError, 0)
		for i, evt := range events {
			select {
			case eventCh <- evt:
			default:
				overflow = append(overflow, model.BatchIngestError{
					Index:         i,
					TransactionID: evt.TransactionID,
					Code:          "queue_full",
					Message:       "stream queue is full; event rejected",
				})
			}
		}
		if len(overflow) > 0 {
			if !enqueueAck(ctx, ackCh, model.BatchIngestResponse{
				AcceptedCount:  0,
				DuplicateCount: 0,
				RejectedCount:  len(overflow),
				Errors:         overflow,
			}, h.stream.ReadTimeout) {
				cancel()
				break
			}
		}
	}

	close(eventCh)
	cancel()
	wg.Wait()
}

func decodeStreamPayload(payload []byte) ([]model.TransactionEvent, error) {
	var batch model.BatchIngestRequest
	if err := json.Unmarshal(payload, &batch); err == nil && len(batch.Events) > 0 {
		return batch.Events, nil
	}

	var evt model.TransactionEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return nil, err
	}
	return []model.TransactionEvent{evt}, nil
}

func enqueueAck(ctx context.Context, ackCh chan<- model.BatchIngestResponse, ack model.BatchIngestResponse, timeout time.Duration) bool {
	waitFor := timeout
	if waitFor <= 0 {
		waitFor = 5 * time.Second
	}

	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	select {
	case ackCh <- ack:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}
