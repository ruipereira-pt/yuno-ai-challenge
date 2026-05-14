package service

import (
	"math"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/model"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

type AlertConfig struct {
	HealthThreshold   float64
	ApprovalThreshold float64
	ErrorThreshold    float64
	SustainedFor      time.Duration
}

type AlertEvaluator struct {
	cfg    AlertConfig
	scorer *Scorer
}

func NewAlertEvaluator(cfg AlertConfig, scorer *Scorer) *AlertEvaluator {
	return &AlertEvaluator{cfg: cfg, scorer: scorer}
}

func (a *AlertEvaluator) Recompute(storeRef *store.WindowStore, now time.Time, maxAge time.Duration, version uint64) {
	snapshots := storeRef.SnapshotBuckets("")
	for psp, buckets := range snapshots {
		incidents, active := a.evaluatePSP(psp, buckets, now, maxAge)
		storeRef.SetIncidents(psp, incidents, active, version)
	}
}

func (a *AlertEvaluator) evaluatePSP(psp string, buckets []store.MinuteBucketView, now time.Time, maxAge time.Duration) ([]model.AlertIncident, *model.AlertIncident) {
	bucketByMinute := make(map[int64]store.MinuteBucketView, len(buckets))
	for _, bucket := range buckets {
		bucketByMinute[bucket.MinuteStart.UTC().Unix()] = bucket
	}

	evalEnd := now.UTC().Truncate(time.Minute)
	evalStart := evalEnd.Add(-maxAge).Add(5 * time.Minute)
	requiredStreak := int(math.Max(1, float64(a.cfg.SustainedFor/time.Minute)))
	streak := 0
	incidents := make([]model.AlertIncident, 0)
	var active *model.AlertIncident

	for sample := evalStart; !sample.After(evalEnd); sample = sample.Add(time.Minute) {
		metrics, hasData := windowMetricsForSample(sample, bucketByMinute)
		if !hasData {
			// No-data samples are treated as unknown: they neither advance the
			// 5-minute degradation streak nor auto-close an already active incident.
			if active == nil {
				streak = 0
			}
			continue
		}

		score := a.scorer.Score(metrics)
		degraded, reason := a.evaluateDegraded(score, metrics.ApprovalRate, metrics.ErrorRate)

		if degraded {
			if active == nil {
				streak++
				if streak >= requiredStreak {
					startedAt := sample.Add(-time.Duration(requiredStreak-1) * time.Minute)
					incidents = append(incidents, model.AlertIncident{
						PSP:       psp,
						StartedAt: startedAt,
						Reason:    reason,
					})
					active = &incidents[len(incidents)-1]
				}
			}
			continue
		}

		streak = 0
		if active != nil {
			endedAt := sample
			active.EndedAt = &endedAt
			active = nil
		}
	}

	return incidents, active
}

func windowMetricsForSample(sampleEnd time.Time, minuteBuckets map[int64]store.MinuteBucketView) (model.WindowMetrics, bool) {
	start := sampleEnd.Add(-5 * time.Minute)
	var metrics model.WindowMetrics
	for minute := start; minute.Before(sampleEnd); minute = minute.Add(time.Minute) {
		bucket, ok := minuteBuckets[minute.UTC().Unix()]
		if !ok {
			continue
		}
		metrics.Total += bucket.Total
		metrics.Approved += bucket.Approved
		metrics.Declined += bucket.Declined
		metrics.Error += bucket.Error
		metrics.AvgResponseTimeMs += float64(bucket.LatencySum)
	}
	if metrics.Total == 0 {
		return model.WindowMetrics{}, false
	}
	metrics.ApprovalRate = float64(metrics.Approved) / float64(metrics.Total)
	metrics.ErrorRate = float64(metrics.Error) / float64(metrics.Total)
	metrics.AvgResponseTimeMs /= float64(metrics.Total)
	return metrics, true
}

func (a *AlertEvaluator) evaluateDegraded(score float64, approvalRate float64, errorRate float64) (bool, string) {
	if score < a.cfg.HealthThreshold {
		return true, "health"
	}
	if approvalRate < a.cfg.ApprovalThreshold {
		return true, "approval"
	}
	if errorRate > a.cfg.ErrorThreshold {
		return true, "error"
	}
	return false, ""
}
