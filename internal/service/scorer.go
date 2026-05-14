package service

import (
	"math"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

type Scorer struct{}

func NewScorer() *Scorer {
	return &Scorer{}
}

func Clamp(v float64, min float64, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func (s *Scorer) Score(m model.WindowMetrics) float64 {
	approvalScore := Clamp(m.ApprovalRate*100.0, 0, 100)
	errorScore := Clamp((1.0-m.ErrorRate)*100.0, 0, 100)

	latencyScore := 100.0
	if m.AvgResponseTimeMs > 200 {
		latencyScore = 100.0 - ((m.AvgResponseTimeMs-200.0)/2800.0)*100.0
	}
	latencyScore = Clamp(latencyScore, 0, 100)

	score := 0.60*approvalScore + 0.25*errorScore + 0.15*latencyScore
	return math.Round(score*10.0) / 10.0
}

func (s *Scorer) IsDegraded(score float64, approvalRate float64, errorRate float64) bool {
	return score < 60 || approvalRate < 0.70 || errorRate > 0.15
}
