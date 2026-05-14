package testdata

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
)

type Generator struct {
	PSPs []string
}

func NewGenerator() *Generator {
	return &Generator{
		PSPs: []string{"PSP_ALPHA", "PSP_BETA", "PSP_GAMMA"},
	}
}

// GenerateTransactions creates deterministic sample traffic with one
// degradation and recovery period for the first PSP.
func (g *Generator) GenerateTransactions(base time.Time, count int) []model.TransactionEvent {
	if count < 1 {
		return []model.TransactionEvent{}
	}
	psps := g.PSPs
	if len(psps) == 0 {
		psps = []string{"acme_pay"}
	}

	events := make([]model.TransactionEvent, 0, count)
	// #nosec G404 -- deterministic synthetic dataset generation; no cryptographic use.
	rnd := rand.New(rand.NewSource(42))

	// ~3h timeline by spreading events every 14 seconds.
	start := base.UTC().Add(-3 * time.Hour)
	degradeStart := start.Add(70 * time.Minute)
	degradeEnd := degradeStart.Add(25 * time.Minute)

	for i := 0; i < count; i++ {
		psp := psps[i%len(psps)]
		ts := start.Add(time.Duration(i*14) * time.Second)

		isDegraded := psp == psps[1] && !ts.Before(degradeStart) && ts.Before(degradeEnd)
		status, latency := sampleProfile(rnd, isDegraded)
		// Slight deterministic jitter to avoid perfectly flat series.
		if latency < 3000 {
			latency += i % 17
		}

		events = append(events, model.TransactionEvent{
			TransactionID:  fmt.Sprintf("tx-%06d", i+1),
			PSP:            psp,
			Status:         status,
			ResponseTimeMs: latency,
			Timestamp:      ts,
		})
	}
	return events
}

func sampleProfile(rnd *rand.Rand, degraded bool) (string, int) {
	p := rnd.Float64()

	if degraded {
		// Degraded mix: approval ~50%, declined ~30%, error ~20%.
		switch {
		case p < 0.50:
			return model.StatusApproved, 1500 + rnd.Intn(1501)
		case p < 0.80:
			return model.StatusDeclined, 1400 + rnd.Intn(1401)
		default:
			return model.StatusError, 1700 + rnd.Intn(1301)
		}
	}

	// Healthy mix: approval ~80%, declined ~16%, error ~4%.
	switch {
	case p < 0.80:
		return model.StatusApproved, 200 + rnd.Intn(601)
	case p < 0.96:
		return model.StatusDeclined, 250 + rnd.Intn(700)
	default:
		return model.StatusError, 400 + rnd.Intn(1100)
	}
}
