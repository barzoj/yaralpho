package app

import (
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/scheduler"
	"go.uber.org/zap"
)

const (
	defaultSchedulerIntervalDuration = 10 * time.Second
	defaultSchedulerMaxRetries       = 5
)

func schedulerOptionsFromConfig(cfg config.Config, logger *zap.Logger) scheduler.Options {
	if logger == nil {
		logger = zap.NewNop()
	}

	interval := defaultSchedulerIntervalDuration
	maxRetries := defaultSchedulerMaxRetries

	if cfg == nil {
		return scheduler.Options{Interval: interval, MaxRetries: maxRetries}
	}

	if val, err := cfg.Get(config.SchedulerIntervalKey); err == nil {
		if parsed, parseErr := time.ParseDuration(strings.TrimSpace(val)); parseErr == nil && parsed > 0 {
			interval = parsed
		} else {
			logger.Warn("invalid scheduler interval; using default", zap.String("value", val), zap.Error(parseErr))
		}
	}

	if val, err := cfg.Get(config.MaxRetriesKey); err == nil {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(val)); parseErr == nil && parsed > 0 {
			maxRetries = parsed
		} else {
			logger.Warn("invalid scheduler max retries; using default", zap.String("value", val), zap.Error(parseErr))
		}
	}

	return scheduler.Options{
		Interval:   interval,
		MaxRetries: maxRetries,
	}
}
