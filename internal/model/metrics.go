package model

import "time"

type WindowMetrics struct {
	Total             int     `json:"total"`
	Approved          int     `json:"approved"`
	Declined          int     `json:"declined"`
	Error             int     `json:"error"`
	ApprovalRate      float64 `json:"approval_rate"`
	ErrorRate         float64 `json:"error_rate"`
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
}

type PSPHealth struct {
	PSP         string                   `json:"psp"`
	HealthScore *float64                 `json:"health_score"`
	NoData      bool                     `json:"no_data"`
	Degraded    bool                     `json:"degraded"`
	Windows     map[string]WindowMetrics `json:"windows"`
}

type HealthResponse struct {
	GeneratedAt time.Time   `json:"generated_at"`
	PSPs        []PSPHealth `json:"psps"`
}

type ComparisonRow struct {
	PSP               string  `json:"psp"`
	HealthScore       float64 `json:"health_score"`
	ApprovalRate      float64 `json:"approval_rate"`
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
}

type ComparisonResponse struct {
	Range struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	} `json:"range"`
	Ranking []ComparisonRow `json:"ranking"`
}
