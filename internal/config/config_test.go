package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEnvOverridesAndTokenPrecedence(t *testing.T) {
	t.Setenv(ConfigPathOverride, "")

	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	require.NoError(t, err)
	content := []byte(`{
        "YARALPHO_MONGODB_URI": "json-uri",
        "YARALPHO_MONGODB_DB": "json-db",
        "YARALPHO_REPO_PATH": "/json/repo",
        "YARALPHO_BD_REPO": "json-bd",
        "YARALPHO_PORT": "7000",
        "GITHUB_TOKEN": "json-gh",
        "YARALPHO_SLACK_WEBHOOK_URL": "https://example.com/json"
    }`)
	require.NoError(t, os.WriteFile(file.Name(), content, 0o644))

	t.Setenv(ConfigPathOverride, file.Name())
	t.Setenv(MongoURIKey, "env-uri")
	t.Setenv(PortKey, "9090")
	t.Setenv(GhTokenKey, "env-gh-token")
	t.Setenv(SlackWebhookKey, "https://example.com/env")

	cfg, err := Load(zap.NewExample())
	require.NoError(t, err)

	uri, err := cfg.Get(MongoURIKey)
	require.NoError(t, err)
	require.Equal(t, "env-uri", uri)

	port, err := cfg.Get(PortKey)
	require.NoError(t, err)
	require.Equal(t, "9090", port)

	token, err := cfg.Get(CopilotTokenKey)
	require.NoError(t, err)
	require.Equal(t, "env-gh-token", token)

	slack, err := cfg.Get(SlackWebhookKey)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/env", slack)
}

func TestPanicOnMissingRequired(t *testing.T) {
	logger := zap.NewExample()

	require.Panics(t, func() {
		_, _ = Load(logger)
	})
}

func TestOptionalSlackNotRequired(t *testing.T) {
	t.Setenv(MongoURIKey, "mongo")
	t.Setenv(MongoDBKey, "db")
	t.Setenv(RepoPathKey, "/repo")
	t.Setenv(BdRepoKey, "bd/repo")
	t.Setenv(CopilotTokenKey, "token")

	var cfg Config
	require.NotPanics(t, func() {
		var err error
		cfg, err = Load(zap.NewExample())
		require.NoError(t, err)
	})

	_, err := cfg.Get(SlackWebhookKey)
	require.Error(t, err)

	port, err := cfg.Get(PortKey)
	require.NoError(t, err)
	require.Equal(t, "8080", port)
}
