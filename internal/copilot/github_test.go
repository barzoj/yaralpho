package copilot

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	githubcopilot "github.com/github/copilot-sdk/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestGitHubStartSessionUsesTokenPrecedence(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "primary")
	t.Setenv("GH_TOKEN", "secondary")
	t.Setenv("GITHUB_TOKEN", "tertiary")

	repoPath := "/tmp/repo"
	fakeSession := &fakeSession{id: "session-123"}
	fakeClient := &fakeClient{session: fakeSession}

	gh := NewGitHub(zaptest.NewLogger(t))
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		fakeClient.opts = opts
		return fakeClient
	}

	sessionID, events, stop, err := gh.StartSession(context.Background(), "hello", repoPath)
	require.NoError(t, err)
	require.Equal(t, fakeSession.id, sessionID)
	require.NotNil(t, events)
	require.NotNil(t, stop)
	require.False(t, fakeClient.stopped)

	require.Equal(t, "primary", fakeClient.opts.GithubToken)
	require.Equal(t, repoPath, fakeClient.opts.Cwd)
	require.NotNil(t, fakeClient.opts.UseLoggedInUser)
	require.False(t, *fakeClient.opts.UseLoggedInUser)

	require.NotNil(t, fakeClient.sessionConfig.OnPermissionRequest)
	require.True(t, fakeClient.sessionConfig.Streaming)
	require.Equal(t, "gpt-5.1-codex-max", fakeClient.sessionConfig.Model)
	require.Equal(t, repoPath, fakeClient.sessionConfig.WorkingDirectory)
	require.Equal(t, "hello", fakeSession.sentPrompt)

	stop()
	require.True(t, fakeClient.stopped)

	_, ok := <-events
	require.False(t, ok, "events channel should be closed after stop")
}

func TestGitHubTokenFallbackOrder(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "fallback-gh")
	t.Setenv("GITHUB_TOKEN", "fallback-github")

	fakeSession := &fakeSession{id: "session-fallback"}
	fakeClient := &fakeClient{session: fakeSession}

	gh := NewGitHub(zaptest.NewLogger(t))
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		fakeClient.opts = opts
		return fakeClient
	}

	sessionID, _, stop, err := gh.StartSession(context.Background(), "hi", "/repo")
	require.NoError(t, err)
	require.Equal(t, fakeSession.id, sessionID)
	require.Equal(t, "fallback-gh", fakeClient.opts.GithubToken)

	stop()
}

func TestGitHubUsesProvidedTokenOverride(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "env-primary")
	provided := "config-token"

	fakeSession := &fakeSession{id: "session-config"}
	fakeClient := &fakeClient{session: fakeSession}

	gh := NewGitHubWithToken(zaptest.NewLogger(t), provided, "config")
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		fakeClient.opts = opts
		return fakeClient
	}

	sessionID, _, stop, err := gh.StartSession(context.Background(), "hi", "/repo")
	require.NoError(t, err)
	require.Equal(t, fakeSession.id, sessionID)
	require.Equal(t, provided, fakeClient.opts.GithubToken)

	stop()
}

func TestGitHubMissingTokenReturnsError(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	fakeClient := &fakeClient{session: &fakeSession{id: "unused"}}
	clientCreated := false

	gh := NewGitHub(zaptest.NewLogger(t))
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		clientCreated = true
		return fakeClient
	}

	_, _, _, err := gh.StartSession(context.Background(), "hello", "/repo")
	require.Error(t, err)
	require.False(t, clientCreated, "client should not be created when token is missing")
}

func TestGitHubForwardsEventsAndStopCloses(t *testing.T) {
	t.Setenv("COPILOT_GITHUB_TOKEN", "token")

	fakeSession := &fakeSession{id: "session-events"}
	fakeClient := &fakeClient{session: fakeSession}

	gh := NewGitHub(zaptest.NewLogger(t))
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		fakeClient.opts = opts
		return fakeClient
	}

	_, events, stop, err := gh.StartSession(context.Background(), "prompt", "/repo")
	require.NoError(t, err)

	msg := "hello"
	event := githubcopilot.SessionEvent{Type: "assistant.message", Data: githubcopilot.Data{Content: &msg}}

	fakeSession.emit(event)

	got := <-events

	expected, err := encodeRawEvent(event)
	require.NoError(t, err)
	require.Equal(t, expected, got)

	stop()

	_, ok := <-events
	require.False(t, ok, "events channel should be closed after stop")
	require.True(t, fakeClient.stopped)

	// Ensure additional events after stop do not panic or send.
	fakeSession.emit(event)
}

func TestGitHubAddsGlobalSkillDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_GITHUB_TOKEN", "token")

	fakeSession := &fakeSession{id: "session-skills"}
	fakeClient := &fakeClient{session: fakeSession}

	gh := NewGitHub(zaptest.NewLogger(t))
	gh.newClient = func(opts *githubcopilot.ClientOptions) copilotClient {
		fakeClient.opts = opts
		return fakeClient
	}

	_, _, stop, err := gh.StartSession(context.Background(), "hi", "/repo")
	require.NoError(t, err)
	require.NotNil(t, fakeClient.sessionConfig)
	require.Contains(t, fakeClient.sessionConfig.SkillDirectories, filepath.Join(home, ".copilot", "skills"))

	stop()
}

func TestResolveCLIPathEnvOverride(t *testing.T) {
	logger := zaptest.NewLogger(t)
	override := "/custom/copilot"
	t.Setenv("COPILOT_CLI_PATH", override)
	t.Setenv("PATH", "")

	path := resolveCLIPath(logger)
	require.Equal(t, override, path)
}

func TestResolveCLIPathPrefersGithubCopilotCliWhenBothPresent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()
	fallbackPath := filepath.Join(dir, "github-copilot-cli")
	createDummyExecutable(t, fallbackPath)
	copilotPath := filepath.Join(dir, "copilot")
	createDummyExecutable(t, copilotPath)
	t.Setenv("PATH", dir)
	t.Setenv("COPILOT_CLI_PATH", "")

	path := resolveCLIPath(logger)
	require.Equal(t, fallbackPath, path)
}

func TestResolveCLIPathUsesCopilotWhenOnlyAliasExists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	dir := t.TempDir()
	copilotPath := filepath.Join(dir, "copilot")
	createDummyExecutable(t, copilotPath)
	t.Setenv("PATH", dir)
	t.Setenv("COPILOT_CLI_PATH", "")

	path := resolveCLIPath(logger)
	require.Equal(t, copilotPath, path)
}

// --- fakes ---

type fakeClient struct {
	opts          *githubcopilot.ClientOptions
	session       *fakeSession
	stopped       bool
	createErr     error
	sessionConfig *githubcopilot.SessionConfig
}

func (f *fakeClient) Stop() error {
	f.stopped = true
	return nil
}

func (f *fakeClient) CreateSession(_ context.Context, config *githubcopilot.SessionConfig) (copilotSession, error) {
	f.sessionConfig = config
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.session, nil
}

type fakeSession struct {
	id         string
	mu         sync.RWMutex
	handlers   []githubcopilot.SessionEventHandler
	sentPrompt string
	sendErr    error
}

func (s *fakeSession) ID() string {
	return s.id
}

func (s *fakeSession) On(handler githubcopilot.SessionEventHandler) func() {
	s.mu.Lock()
	s.handlers = append(s.handlers, handler)
	s.mu.Unlock()

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, h := range s.handlers {
			if &h == &handler {
				s.handlers = append(s.handlers[:i], s.handlers[i+1:]...)
				break
			}
		}
	}
}

func (s *fakeSession) Send(_ context.Context, options githubcopilot.MessageOptions) (string, error) {
	s.sentPrompt = options.Prompt
	if s.sendErr != nil {
		return "", s.sendErr
	}
	return "message", nil
}

func (s *fakeSession) Destroy() error { return nil }

func (s *fakeSession) emit(event githubcopilot.SessionEvent) {
	s.mu.RLock()
	handlers := append([]githubcopilot.SessionEventHandler{}, s.handlers...)
	s.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}

func createDummyExecutable(t *testing.T, path string) {
	t.Helper()
	content := []byte("#!/bin/sh\nexit 0\n")
	require.NoError(t, os.WriteFile(path, content, 0o755))
}
