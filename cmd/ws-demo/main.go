package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func main() {
	var (
		rawURL      = flag.String("url", "ws://localhost:8080/events/stream", "websocket endpoint URL")
		psp         = flag.String("psp", "PSP_ALPHA", "psp name for generated demo event")
		status      = flag.String("status", model.StatusApproved, "status for generated demo event")
		txID        = flag.String("tx", "", "transaction id (auto-generated when empty)")
		responseMs  = flag.Int("response-ms", 220, "response time in ms")
		sendBatch   = flag.Bool("batch", false, "send as {\"events\":[...]} payload instead of single event")
		scenario    = flag.String("scenario", "single", "demo scenario: single|degraded")
		readTimeout = flag.Duration("timeout", 5*time.Second, "timeout waiting for ack")
	)
	flag.Parse()

	parsedURL, err := url.Parse(*rawURL)
	if err != nil {
		log.Fatalf("invalid -url: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(parsedURL.String(), nil)
	if err != nil {
		log.Fatalf("connect websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(*readTimeout)); err != nil {
		log.Fatalf("set write deadline: %v", err)
	}

	now := time.Now().UTC()
	events := buildEvents(*scenario, now, *txID, *psp, *status, *responseMs)
	if len(events) == 0 {
		log.Fatalf("scenario %q produced no events", *scenario)
	}

	var payload any = events[0]
	if *sendBatch || *scenario == "degraded" {
		payload = model.BatchIngestRequest{Events: events}
	}

	if err := conn.WriteJSON(payload); err != nil {
		log.Fatalf("send event: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(*readTimeout)); err != nil {
		log.Fatalf("set read deadline: %v", err)
	}

	var ack model.BatchIngestResponse
	if err := conn.ReadJSON(&ack); err != nil {
		log.Fatalf("read ack: %v", err)
	}

	rawAck, err := json.MarshalIndent(ack, "", "  ")
	if err != nil {
		log.Fatalf("marshal ack: %v", err)
	}

	fmt.Printf("%s\n", rawAck)
}

func buildEvents(scenario string, now time.Time, txID string, psp string, status string, responseMs int) []model.TransactionEvent {
	switch scenario {
	case "single":
		id := txID
		if id == "" {
			id = fmt.Sprintf("ws-demo-%d", now.UnixNano())
		}
		return []model.TransactionEvent{
			{
				TransactionID:  id,
				PSP:            psp,
				Status:         status,
				ResponseTimeMs: responseMs,
				Timestamp:      now,
			},
		}
	case "degraded":
		baseID := txID
		if baseID == "" {
			baseID = fmt.Sprintf("ws-degraded-%d", now.UnixNano())
		}

		events := make([]model.TransactionEvent, 0, 8)
		for i := 0; i < 8; i++ {
			events = append(events, model.TransactionEvent{
				TransactionID:  fmt.Sprintf("%s-%02d", baseID, i),
				PSP:            psp,
				Status:         model.StatusError,
				ResponseTimeMs: 2200,
				Timestamp:      now.Add(-time.Duration(8-i) * time.Minute),
			})
		}
		return events
	default:
		return nil
	}
}
