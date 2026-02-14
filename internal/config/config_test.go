package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
        "YARALPHO_SLACK_WEBHOOK_URL": "https://example.com/json",
        "YARALPHO_MAX_RETRIES": "3",
        "YARALPHO_SCHEDULER_INTERVAL": "99s",
        "YARALPHO_RESTART_WAIT_TIMEOUT": "45s",
        "YARALPHO_EXECUTION_TASK_PROMPT": "json execution",
        "YARALPHO_VERIFICATION_TASK_PROMPT": "json verification"
    }`)
	require.NoError(t, os.WriteFile(file.Name(), content, 0o644))

	t.Setenv(ConfigPathOverride, file.Name())
	t.Setenv(MongoURIKey, "env-uri")
	t.Setenv(PortKey, "9090")
	t.Setenv(SlackWebhookKey, "https://example.com/env")
	t.Setenv(MaxRetriesKey, "7")
	t.Setenv(SchedulerIntervalKey, "15s")
	t.Setenv(RestartWaitTimeoutKey, "20s")
	t.Setenv(ExecutionTaskPromptKey, "env execution prompt")
	t.Setenv(VerificationTaskPromptKey, "env verification prompt")

	cfg, err := Load(zap.NewExample())
	require.NoError(t, err)

	uri, err := cfg.Get(MongoURIKey)
	require.NoError(t, err)
	require.Equal(t, "env-uri", uri)

	port, err := cfg.Get(PortKey)
	require.NoError(t, err)
	require.Equal(t, "9090", port)

	slack, err := cfg.Get(SlackWebhookKey)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/env", slack)

	maxRetries, err := cfg.Get(MaxRetriesKey)
	require.NoError(t, err)
	require.Equal(t, "7", maxRetries)

	interval, err := cfg.Get(SchedulerIntervalKey)
	require.NoError(t, err)
	require.Equal(t, "15s", interval)

	restartWait, err := cfg.Get(RestartWaitTimeoutKey)
	require.NoError(t, err)
	require.Equal(t, "20s", restartWait)

	execPrompt, err := cfg.Get(ExecutionTaskPromptKey)
	require.NoError(t, err)
	require.Equal(t, "env execution prompt", execPrompt)

	verifyPrompt, err := cfg.Get(VerificationTaskPromptKey)
	require.NoError(t, err)
	require.Equal(t, "env verification prompt", verifyPrompt)
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
	t.Setenv(PortKey, "")
	t.Setenv(SlackWebhookKey, "")
	t.Setenv(MaxRetriesKey, "")
	t.Setenv(SchedulerIntervalKey, "")
	t.Setenv(RestartWaitTimeoutKey, "")
	t.Setenv(ExecutionTaskPromptKey, "")
	t.Setenv(VerificationTaskPromptKey, "")

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

	maxRetries, err := cfg.Get(MaxRetriesKey)
	require.NoError(t, err)
	require.Equal(t, "5", maxRetries)

	interval, err := cfg.Get(SchedulerIntervalKey)
	require.NoError(t, err)
	require.Equal(t, "10s", interval)

	restartWait, err := cfg.Get(RestartWaitTimeoutKey)
	require.NoError(t, err)
	require.Equal(t, "30s", restartWait)

	execPrompt, err := cfg.Get(ExecutionTaskPromptKey)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(defaultExecutionTaskPrompt), strings.TrimSpace(execPrompt))

	verifyPrompt, err := cfg.Get(VerificationTaskPromptKey)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(defaultVerificationTaskPrompt), strings.TrimSpace(verifyPrompt))
}

func TestLoadWithPath_WarnsOnEnvOverride(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	require.NoError(t, err)

	content := []byte(`{
		"YARALPHO_MONGODB_URI": "json-uri",
		"YARALPHO_MONGODB_DB": "json-db",
		"YARALPHO_REPO_PATH": "/json/repo",
		"YARALPHO_BD_REPO": "json-bd",
		"YARALPHO_PORT": "8080",
		"YARALPHO_SLACK_WEBHOOK_URL": "https://example.com/json"
	}`)
	require.NoError(t, os.WriteFile(file.Name(), content, 0o644))

	t.Setenv(MongoURIKey, "env-uri")

	core, recorded := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	_, err = LoadWithPath(logger, file.Name())
	require.NoError(t, err)

	entries := recorded.All()
	require.Len(t, entries, 1)
	require.Equal(t, "environment overrides config file value", entries[0].Message)
	fields := entries[0].ContextMap()
	require.Equal(t, MongoURIKey, fields["key"])
	require.Equal(t, "json-uri", fields["file_value"])
	require.Equal(t, "env-uri", fields["env_value"])
}

func TestLoadWithPath_NoWarningWhenOnlyOneSourceProvidesValue(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	require.NoError(t, err)

	content := []byte(`{
		"YARALPHO_MONGODB_DB": "json-db",
		"YARALPHO_REPO_PATH": "/json/repo",
		"YARALPHO_BD_REPO": "json-bd",
		"YARALPHO_PORT": "8080",
		"COPILOT_GITHUB_TOKEN": "json-token"
	}`)
	require.NoError(t, os.WriteFile(file.Name(), content, 0o644))

	t.Setenv(MongoURIKey, "env-uri")

	core, recorded := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	_, err = LoadWithPath(logger, file.Name())
	require.NoError(t, err)

	require.Len(t, recorded.All(), 0)
}

func TestLoadWithPath_SecretOverrideDoesNotLogRawValues(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "config-*.json")
	require.NoError(t, err)

	content := []byte(`{
		"YARALPHO_MONGODB_URI": "json-uri",
		"YARALPHO_MONGODB_DB": "json-db",
		"YARALPHO_REPO_PATH": "/json/repo",
		"YARALPHO_BD_REPO": "json-bd",
		"YARALPHO_PORT": "8080",
		"YARALPHO_SLACK_WEBHOOK_URL": "https://example.com/json-secret"
	}`)
	require.NoError(t, os.WriteFile(file.Name(), content, 0o644))

	t.Setenv(SlackWebhookKey, "https://example.com/env-secret")

	core, recorded := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	_, err = LoadWithPath(logger, file.Name())
	require.NoError(t, err)

	entries := recorded.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, SlackWebhookKey, fields["key"])
	require.Equal(t, true, fields["env_overrides_file"])
	_, hasFileValue := fields["file_value"]
	_, hasEnvValue := fields["env_value"]
	require.False(t, hasFileValue)
	require.False(t, hasEnvValue)
	require.NotContains(t, entries[0].Message, "json-secret")
	require.NotContains(t, entries[0].Message, "env-secret")
}
