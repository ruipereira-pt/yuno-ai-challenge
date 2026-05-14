package model

import "time"

type AlertIncident struct {
	PSP       string     `json:"psp"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Reason    string     `json:"reason"`
}

type AlertsResponse struct {
	PSP    string          `json:"psp,omitempty"`
	Events []AlertIncident `json:"events"`
}

type ErrorResponse struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Details map[string]string `json:"details,omitempty"`
	} `json:"error"`
}
