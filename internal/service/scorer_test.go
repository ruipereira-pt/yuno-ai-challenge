package service

import (
	"testing"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

func TestScoreTableDriven(t *testing.T) {
	scorer := NewScorer()

	tests := []struct {
		name     string
		metrics  model.WindowMetrics
		expected float64
	}{
		{
			name: "healthy high approval low error low latency",
			metrics: model.WindowMetrics{
				ApprovalRate:      0.95,
				ErrorRate:         0.02,
				AvgResponseTimeMs: 250,
			},
			expected: 96.2,
		},
		{
			name: "degraded low approval high error high latency",
			metrics: model.WindowMetrics{
				ApprovalRate:      0.50,
				ErrorRate:         0.20,
				AvgResponseTimeMs: 2000,
			},
			expected: 55.4,
		},
		{
			name: "latency below floor clamps to 100",
			metrics: model.WindowMetrics{
				ApprovalRate:      0.80,
				ErrorRate:         0.04,
				AvgResponseTimeMs: 100,
			},
			expected: 87,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scorer.Score(tc.metrics)
			if got != tc.expected {
				t.Fatalf("expected %.1f, got %.1f", tc.expected, got)
			}
		})
	}
}

func TestIsDegradedTableDriven(t *testing.T) {
	scorer := NewScorer()

	tests := []struct {
		name         string
		score        float64
		approvalRate float64
		errorRate    float64
		expected     bool
	}{
		{"healthy", 80, 0.85, 0.03, false},
		{"degrade by score", 59.9, 0.85, 0.03, true},
		{"degrade by approval", 85, 0.69, 0.03, true},
		{"degrade by error", 85, 0.85, 0.16, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := scorer.IsDegraded(tc.score, tc.approvalRate, tc.errorRate)
			if got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}
