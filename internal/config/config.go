package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"go.uber.org/zap"
)

// Config exposes read-only access to configuration values sourced from
// environment variables with an optional JSON fallback.
type Config interface {
	Get(key string) (string, error)
}

// Keys used throughout the application. They mirror the expected env vars.
const (
	MongoURIKey               = "YARALPHO_MONGODB_URI"
	MongoDBKey                = "YARALPHO_MONGODB_DB"
	RepoPathKey               = "YARALPHO_REPO_PATH"
	BdRepoKey                 = "YARALPHO_BD_REPO"
	PortKey                   = "YARALPHO_PORT"
	SlackWebhookKey           = "YARALPHO_SLACK_WEBHOOK_URL"
	CopilotTokenKey           = "COPILOT_GITHUB_TOKEN"
	GhTokenKey                = "GH_TOKEN"
	GithubTokenKey            = "GITHUB_TOKEN"
	MaxRetriesKey             = "YARALPHO_MAX_RETRIES"
	SchedulerIntervalKey      = "YARALPHO_SCHEDULER_INTERVAL"
	RestartWaitTimeoutKey     = "YARALPHO_RESTART_WAIT_TIMEOUT"
	ExecutionTaskPromptKey    = "YARALPHO_EXECUTION_TASK_PROMPT"
	VerificationTaskPromptKey = "YARALPHO_VERIFICATION_TASK_PROMPT"
	ConfigPathOverride        = "RALPH_CONFIG"
)

const (
	defaultConfigPath          = "config.json"
	defaultMaxRetries          = "5"
	defaultSchedulerInterval   = "10s"
	defaultRestartWaitTimeout  = "30s"
	defaultExecutionTaskPrompt = `
	You are an execution agent. There is no human available to assist you. You need to complete the assigned task by following the instructions and working with the tools at your disposal.
	Make sure to read staff-software-engineer skill if writing any code.
	If writing frontend code, make sure to read frontend-engineer skill.
	Task is completed when you have made a commit with a message that mentions the task name, repo working tree MUST be clean, otherwise the task is not completed.
	Here is the issue to work on: %s

	`
	defaultVerificationTaskPrompt = `
	You are a verification agent. You are given the following task to verify work results of another coding agent:

Task name: %s

Follow these steps and return only one JSON object in your final assistant message:

1) Run: git status --short
   - If any tracked/untracked files are listed, set:
     {
       "status": "failure",
       "reason": "working_tree_not_clean",
       "details": "Brief list of files and changes (escape quotes/backslashes)"
     }
   - Stop after emitting this JSON.

2) If clean:

   - Get last commit info: git log -1 --oneline
     * If no commits or the last commit message does NOT mention the task name exactly:
       {
         "status": "success",
         "reason": "<last commit hash or 'none'>",
         "details": "no commit done"
       }
     * Otherwise:
       - Summarize what changed in the last commit in plain words (no bullets, keep concise).
       - Return:
         {
           "status": "success",
           "reason": "<last commit hash>",
           "details": "<plain summary of last commit changes,  (escape quotes/backslashes)>"
         }

Rules:
- Final assistant message must be exactly one JSON object, no code fences, no extra text.
- Escape JSON specials: replace " with \", \ with \\, newlines with \n.
- Do not include Markdown. No extra keys. No trailing commas.
	`
)

var requiredKeys = []string{
	MongoURIKey,
	MongoDBKey,
	RepoPathKey,
	BdRepoKey,
	PortKey,
	CopilotTokenKey,
}

var envOverrideKeys = []string{
	MongoURIKey,
	MongoDBKey,
	RepoPathKey,
	BdRepoKey,
	PortKey,
	SlackWebhookKey,
	CopilotTokenKey,
	GhTokenKey,
	GithubTokenKey,
	MaxRetriesKey,
	SchedulerIntervalKey,
	RestartWaitTimeoutKey,
	ExecutionTaskPromptKey,
	VerificationTaskPromptKey,
}

var secretKeys = map[string]struct{}{
	CopilotTokenKey: {},
	GhTokenKey:      {},
	GithubTokenKey:  {},
}

// LoggableKeys lists configuration keys safe to emit in logs. Token values are
// intentionally excluded to avoid leaking credentials.
func LoggableKeys() []string {
	return []string{
		MongoURIKey,
		MongoDBKey,
		RepoPathKey,
		BdRepoKey,
		PortKey,
		SlackWebhookKey,
		MaxRetriesKey,
		SchedulerIntervalKey,
		RestartWaitTimeoutKey,
		ExecutionTaskPromptKey,
		VerificationTaskPromptKey,
	}
}

// Load builds a Config using environment variables first, falling back to a
// JSON file. Missing required keys cause a zap.Panic.
func Load(logger *zap.Logger) (Config, error) {
	return LoadWithPath(logger, "")
}

// LoadWithPath allows overriding the JSON config path (RALPH_CONFIG still wins).
func LoadWithPath(logger *zap.Logger, path string) (Config, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	cfgPath := strings.TrimSpace(path)
	if envPath, ok := lookupEnv(ConfigPathOverride); ok {
		cfgPath = envPath
	}
	if cfgPath == "" {
		cfgPath = defaultConfigPath
	}

	jsonValues, err := loadJSON(cfgPath)
	if err != nil {
		return nil, err
	}

	values := make(map[string]string)
	// Seed with JSON values (treated as fallback)
	for k, v := range jsonValues {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			values[k] = trimmed
		}
	}

	// Apply env overrides
	for _, key := range envOverrideKeys {
		if val, ok := lookupEnv(key); ok {
			if fileValue, fileHasValue := jsonValues[key]; fileHasValue && strings.TrimSpace(fileValue) != "" {
				logEnvOverrideCollision(logger, key, strings.TrimSpace(fileValue), val)
			}
			values[key] = val
		}
	}

	// Port default
	if strings.TrimSpace(values[PortKey]) == "" {
		values[PortKey] = "8080"
	}

	// Token precedence: COPILOT_GITHUB_TOKEN > GH_TOKEN > GITHUB_TOKEN
	values[CopilotTokenKey] = firstNonEmpty(
		values[CopilotTokenKey],
		values[GhTokenKey],
		values[GithubTokenKey],
	)

	if strings.TrimSpace(values[MaxRetriesKey]) == "" {
		values[MaxRetriesKey] = defaultMaxRetries
	}
	if strings.TrimSpace(values[SchedulerIntervalKey]) == "" {
		values[SchedulerIntervalKey] = defaultSchedulerInterval
	}
	if strings.TrimSpace(values[RestartWaitTimeoutKey]) == "" {
		values[RestartWaitTimeoutKey] = defaultRestartWaitTimeout
	}
	if strings.TrimSpace(values[ExecutionTaskPromptKey]) == "" {
		values[ExecutionTaskPromptKey] = defaultExecutionTaskPrompt
	}
	if strings.TrimSpace(values[VerificationTaskPromptKey]) == "" {
		values[VerificationTaskPromptKey] = defaultVerificationTaskPrompt
	}

	missing := missingKeys(values, requiredKeys)
	if len(missing) > 0 {
		logger.Panic("missing required configuration", zap.Strings("keys", missing))
	}

	return &mapConfig{values: values}, nil
}

func loadJSON(path string) (map[string]string, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]string{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return parsed, nil
}

func missingKeys(values map[string]string, keys []string) []string {
	missing := make([]string, 0)
	for _, key := range keys {
		if strings.TrimSpace(values[key]) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func lookupEnv(key string) (string, bool) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return "", false
	}
	return trimmed, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func logEnvOverrideCollision(logger *zap.Logger, key, fileValue, envValue string) {
	if isSecretKey(key) {
		logger.Warn(
			"environment overrides config file value",
			zap.String("key", key),
			zap.Bool("env_overrides_file", true),
		)
		return
	}

	logger.Warn(
		"environment overrides config file value",
		zap.String("key", key),
		zap.String("file_value", fileValue),
		zap.String("env_value", envValue),
	)
}

func isSecretKey(key string) bool {
	_, ok := secretKeys[key]
	return ok
}

type mapConfig struct {
	values map[string]string
}

func (c *mapConfig) Get(key string) (string, error) {
	if val, ok := c.values[key]; ok && strings.TrimSpace(val) != "" {
		return val, nil
	}
	return "", fmt.Errorf("config key %s not found", key)
}
