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
	MongoURIKey        = "YARALPHO_MONGODB_URI"
	MongoDBKey         = "YARALPHO_MONGODB_DB"
	RepoPathKey        = "YARALPHO_REPO_PATH"
	BdRepoKey          = "YARALPHO_BD_REPO"
	PortKey            = "YARALPHO_PORT"
	SlackWebhookKey    = "YARALPHO_SLACK_WEBHOOK_URL"
	CopilotTokenKey    = "COPILOT_GITHUB_TOKEN"
	GhTokenKey         = "GH_TOKEN"
	GithubTokenKey     = "GITHUB_TOKEN"
	ConfigPathOverride = "RALPH_CONFIG"
)

const defaultConfigPath = "config.json"

var requiredKeys = []string{
	MongoURIKey,
	MongoDBKey,
	RepoPathKey,
	BdRepoKey,
	PortKey,
	CopilotTokenKey,
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
	for _, key := range []string{
		MongoURIKey,
		MongoDBKey,
		RepoPathKey,
		BdRepoKey,
		PortKey,
		SlackWebhookKey,
		CopilotTokenKey,
		GhTokenKey,
		GithubTokenKey,
	} {
		if val, ok := lookupEnv(key); ok {
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

type mapConfig struct {
	values map[string]string
}

func (c *mapConfig) Get(key string) (string, error) {
	if val, ok := c.values[key]; ok && strings.TrimSpace(val) != "" {
		return val, nil
	}
	return "", fmt.Errorf("config key %s not found", key)
}
