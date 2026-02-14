package app

import (
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/scheduler"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSchedulerOptionsFromConfig_UsesConfigValues(t *testing.T) {
	cfg := fakeConfig{
		config.SchedulerIntervalKey: "250ms",
		config.MaxRetriesKey:        "9",
	}

	opts := schedulerOptionsFromConfig(cfg, zap.NewNop())

	require.Equal(t, 250*time.Millisecond, opts.Interval)
	require.Equal(t, 9, opts.MaxRetries)
}

func TestSchedulerOptionsFromConfig_DefaultsOnMissing(t *testing.T) {
	opts := schedulerOptionsFromConfig(fakeConfig{}, zap.NewNop())
	require.Equal(t, defaultSchedulerIntervalDuration, opts.Interval)
	require.Equal(t, defaultSchedulerMaxRetries, opts.MaxRetries)
}

func TestSchedulerOptionsFromConfig_InvalidValuesWarn(t *testing.T) {
	cfg := fakeConfig{
		config.SchedulerIntervalKey: "abc",
		config.MaxRetriesKey:        "bad",
	}

	core, recorded := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	opts := schedulerOptionsFromConfig(cfg, logger)

	require.Equal(t, defaultSchedulerIntervalDuration, opts.Interval)
	require.Equal(t, defaultSchedulerMaxRetries, opts.MaxRetries)
	require.Len(t, recorded.All(), 2)
	require.Equal(t, "invalid scheduler interval; using default", recorded.All()[0].Message)
	require.Equal(t, "invalid scheduler max retries; using default", recorded.All()[1].Message)
}

func TestSchedulerOptionsFromConfig_NilConfig(t *testing.T) {
	opts := schedulerOptionsFromConfig(nil, zap.NewNop())
	require.Equal(t, defaultSchedulerIntervalDuration, opts.Interval)
	require.Equal(t, defaultSchedulerMaxRetries, opts.MaxRetries)
	require.IsType(t, scheduler.Options{}, opts)
}
