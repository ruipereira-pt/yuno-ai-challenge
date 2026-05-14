package config

import (
	"os"
	"strconv"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/service"
)

type Config struct {
	HTTPAddr      string
	MaxEventAge   time.Duration
	MaxFutureSkew time.Duration
	Alert         service.AlertConfig
	WS            WSConfig
}

type WSConfig struct {
	Enabled       bool
	MaxFrameBytes int64
	ReadTimeout   time.Duration
	BatchSize     int
	FlushInterval time.Duration
	QueueSize     int
}

func Load() Config {
	return Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		MaxEventAge:   getEnvDuration("MAX_EVENT_AGE", 180*time.Minute),
		MaxFutureSkew: getEnvDuration("MAX_FUTURE_SKEW", 2*time.Minute),
		Alert: service.AlertConfig{
			HealthThreshold:   getEnvFloat("ALERT_HEALTH_THRESHOLD", 60),
			ApprovalThreshold: getEnvFloat("ALERT_APPROVAL_LT", 0.70),
			ErrorThreshold:    getEnvFloat("ALERT_ERROR_GT", 0.15),
			SustainedFor:      getEnvDuration("ALERT_MIN_DURATION", 5*time.Minute),
		},
		WS: WSConfig{
			Enabled:       getEnvBool("WS_INGEST_ENABLED", false),
			MaxFrameBytes: int64(getEnvInt("WS_MAX_FRAME_BYTES", 1024*1024)),
			ReadTimeout:   getEnvDuration("WS_READ_TIMEOUT", 30*time.Second),
			BatchSize:     getEnvInt("WS_BATCH_SIZE", 50),
			FlushInterval: getEnvDuration("WS_FLUSH_INTERVAL", 1*time.Second),
			QueueSize:     getEnvInt("WS_QUEUE_SIZE", 1000),
		},
	}
}

func getEnv(key string, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func getEnvFloat(key string, fallback float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return fallback
	}
	return parsed
}
