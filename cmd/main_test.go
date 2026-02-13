package main

import (
	"context"
	"testing"

	"github.com/barzoj/yaralpho/internal/app"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestParseCLIOptionsAgent(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantAgent string
		wantErr   string
	}{
		{
			name:      "default agent is codex",
			args:      []string{},
			wantAgent: "codex",
		},
		{
			name:      "github accepted",
			args:      []string{"--agent=github"},
			wantAgent: "github",
		},
		{
			name:      "codex accepted",
			args:      []string{"--agent=codex"},
			wantAgent: "codex",
		},
		{
			name:    "invalid agent rejected",
			args:    []string{"--agent=invalid"},
			wantErr: "invalid agent",
		},
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
			require.Equal(t, tc.wantAgent, opts.Agent)
		})
	}
}

func TestBuildApplicationPassesAgentToBuildOptions(t *testing.T) {
	origBuildWithOptions := buildWithOptions
	t.Cleanup(func() {
		buildWithOptions = origBuildWithOptions
	})

	var gotAgent string
	buildWithOptions = func(ctx context.Context, logger *zap.Logger, cfg config.Config, opts app.BuildOptions) (*app.App, error) {
		gotAgent = opts.Agent
		return &app.App{}, nil
	}

	_, err := buildApplication(context.Background(), zap.NewNop(), nil, "github")
	require.NoError(t, err)
	require.Equal(t, "github", gotAgent)
}
