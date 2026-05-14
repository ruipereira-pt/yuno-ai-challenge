package model

import "time"

const (
	StatusApproved = "approved"
	StatusDeclined = "declined"
	StatusError    = "error"
)

type TransactionEvent struct {
	TransactionID  string    `json:"transaction_id"`
	PSP            string    `json:"psp"`
	Status         string    `json:"status"`
	ResponseTimeMs int       `json:"response_time_ms"`
	Timestamp      time.Time `json:"timestamp"`
}

type BatchIngestRequest struct {
	Events []TransactionEvent `json:"events"`
}

type BatchIngestResponse struct {
	AcceptedCount  int                `json:"accepted_count"`
	DuplicateCount int                `json:"duplicate_count"`
	RejectedCount  int                `json:"rejected_count"`
	Errors         []BatchIngestError `json:"errors"`
}

type BatchIngestError struct {
	Index         int    `json:"index"`
	TransactionID string `json:"transaction_id"`
	Code          string `json:"code"`
	Message       string `json:"message"`
}
