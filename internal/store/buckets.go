package store

import "time"

type minuteBucket struct {
	MinuteStart time.Time
	Total       int
	Approved    int
	Declined    int
	Error       int
	LatencySum  int64
}

type MinuteBucketView struct {
	MinuteStart time.Time
	Total       int
	Approved    int
	Declined    int
	Error       int
	LatencySum  int64
}

func (b *minuteBucket) add(status string, latencyMs int) {
	b.Total++
	b.LatencySum += int64(latencyMs)
	switch status {
	case "approved":
		b.Approved++
	case "declined":
		b.Declined++
	case "error":
		b.Error++
	}
}
