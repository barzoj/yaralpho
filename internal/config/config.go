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
	PortKey                   = "YARALPHO_PORT"
	SlackWebhookKey           = "YARALPHO_SLACK_WEBHOOK_URL"
	MaxRetriesKey             = "YARALPHO_MAX_RETRIES"
	SchedulerIntervalKey      = "YARALPHO_SCHEDULER_INTERVAL"
	RestartWaitTimeoutKey     = "YARALPHO_RESTART_WAIT_TIMEOUT"
	ExecutionTaskPromptKey    = "YARALPHO_EXECUTION_TASK_PROMPT"
	VerificationTaskPromptKey = "YARALPHO_VERIFICATION_TASK_PROMPT"
	TaskExecTimeoutKey        = "YARALPHO_TASK_EXEC_TIMEOUT"
	TaskVerifyTimeoutKey      = "YARALPHO_TASK_VERIFY_TIMEOUT"
	ConfigPathOverride        = "RALPH_CONFIG"
)

const (
	defaultConfigPath          = "config.json"
	defaultMaxRetries          = "5"
	defaultSchedulerInterval   = "10s"
	defaultRestartWaitTimeout  = "20m"
	defaultTaskExecTimeout     = "20m"
	defaultTaskVerifyTimeout   = "20m"
	defaultExecutionTaskPrompt = `
You are an execution agent. No human will answer questions. Finish the task end-to-end.

Environment: shell access is available; use bd CLI for tracking.

Protocol (do not skip steps):
1) Read the task (and related tasks/epics if referenced) to understand scope/acceptance. If anything is missing, say exactly what is missing and stop.
2) Write a short execution plan with concrete steps + planned verification.
3) Claim the task: ensure status is in_progress (run 'bd update <id> --status in_progress' unless already in that state).
4) Gather only needed context (code/docs).
5) Implement changes.
6) Verification: run the relevant tests/checks; state what "done" means and show command outputs.

7) Git hygiene before finalizing:
   - ensure 'git status' clean before commit
   - commit message must mention the task ID/name
   - avoid committing build caches, large data, secrets, temp files

8) Finalization sequence (run in order):
   - 'git pull --rebase'
   - 'bd sync'
   - 'git push'
   - confirm 'git status' reports "up to date with origin"

9) Close the task ONLY IF implementation and verification are complete and push succeeded:
   - run 'bd close <id>'
   - confirm it succeeded
   - if not complete, leave open and state what is pending/blocking

10) Report concisely: what changed, tests run/results, git status, whether 'bd close' executed (or why not).

Rules:
- Prefer correctness over speed; explain any deviations.
- If blocked (missing access, failing tests you cannot fix), stop and state the block clearly.
- Use staff-software-engineer for code; frontend-engineer for UI work.
- Never end the session with unpushed commits; only close when truly done.


Task to execute: %s

`
	defaultVerificationTaskPrompt = `
You are a verification agent. Verify the work of another coding agent and report a single JSON result.

Checks (stop at first failure; otherwise continue):
1) Working tree clean: run git status --short. If any tracked or untracked files appear, output:
   {
     "status": "failure",
     "reason": "working_tree_not_clean",
     "details": "Brief list of files and changes (escape quotes/backslashes)"
   }

2) Task closed: confirm the task is marked closed in the tracker. If not closed, output:
   {
     "status": "failure",
     "reason": "task_not_closed",
     "details": "Current task status"
   }

3) Commit requirement and last commit:
   - Get last commit: git log -1 --oneline (hash and message).
   - If the message mentions the task name/ID, summarize the last commit in plain words (no bullets) and output:
     {
       "status": "success",
       "reason": "<last commit hash>",
       "details": "<plain summary, escape quotes/backslashes>"
     }
   - If it does NOT mention the task:
     * Read the task description. If the task required code/config/doc changes and no commit references it, output:
       {
         "status": "failure",
         "reason": "commit_missing",
         "details": "Task needs commit but none references it"
       }
     * If the task did not require a commit (pure verification/admin), output:
       {
         "status": "success",
         "reason": "no_commit_required",
         "details": "Task completed without commit"
       }

Rules:
- Final assistant message must be exactly one JSON object, no code fences, no extra text.
- Escape JSON specials: replace " with \", \ with \\, newlines with \n.
- Do not include Markdown. No extra keys. No trailing commas.

Task name: %s
	`
)

var requiredKeys = []string{
	MongoURIKey,
	MongoDBKey,
	RepoPathKey,
	PortKey,
}

var envOverrideKeys = []string{
	MongoURIKey,
	MongoDBKey,
	RepoPathKey,
	PortKey,
	SlackWebhookKey,
	MaxRetriesKey,
	SchedulerIntervalKey,
	RestartWaitTimeoutKey,
	ExecutionTaskPromptKey,
	VerificationTaskPromptKey,
	TaskExecTimeoutKey,
	TaskVerifyTimeoutKey,
}

var secretKeys = map[string]struct{}{
	SlackWebhookKey: {},
}

// LoggableKeys lists configuration keys safe to emit in logs. Secrets are
// intentionally excluded to avoid leaking credentials.
func LoggableKeys() []string {
	return []string{
		MongoURIKey,
		MongoDBKey,
		RepoPathKey,
		PortKey,
		MaxRetriesKey,
		SchedulerIntervalKey,
		RestartWaitTimeoutKey,
		ExecutionTaskPromptKey,
		VerificationTaskPromptKey,
		TaskExecTimeoutKey,
		TaskVerifyTimeoutKey,
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

	if strings.TrimSpace(values[MaxRetriesKey]) == "" {
		values[MaxRetriesKey] = defaultMaxRetries
	}
	if strings.TrimSpace(values[SchedulerIntervalKey]) == "" {
		values[SchedulerIntervalKey] = defaultSchedulerInterval
	}
	if strings.TrimSpace(values[RestartWaitTimeoutKey]) == "" {
		values[RestartWaitTimeoutKey] = defaultRestartWaitTimeout
	}
	if strings.TrimSpace(values[TaskExecTimeoutKey]) == "" {
		values[TaskExecTimeoutKey] = defaultTaskExecTimeout
	}
	if strings.TrimSpace(values[TaskVerifyTimeoutKey]) == "" {
		values[TaskVerifyTimeoutKey] = defaultTaskVerifyTimeout
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
