package main

import (
	"context"
	"testing"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestParseCLIOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantLog string
		wantErr string
	}{
		{name: "default debug-level is info", args: []string{}, wantLog: "info"},
		{name: "warn accepted", args: []string{"--debug-level=warn"}, wantLog: "warn"},
		{name: "debug accepted", args: []string{"--debug-level=debug"}, wantLog: "debug"},
		{name: "invalid level rejected", args: []string{"--debug-level=nope"}, wantErr: "invalid debug-level"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseCLIOptions(tc.args)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantLog, opts.LogLevel)
		})
	}
}

func TestBuildApplicationCallsBuild(t *testing.T) {
	origBuildApp := buildApp
	t.Cleanup(func() { buildApp = origBuildApp })

	called := false
	buildApp = func(ctx context.Context, logger *zap.Logger, cfg config.Config) (*app.App, error) {
		called = true
		return &app.App{}, nil
	}

	_, err := buildApplication(context.Background(), zap.NewNop(), nil)
	require.NoError(t, err)
	require.True(t, called)
}
