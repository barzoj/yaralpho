package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	githubcopilot "github.com/github/copilot-sdk/go"
	"go.uber.org/zap"
)

const eventBufferSize = 32

var _ Client = (*GitHub)(nil)

// GitHub implements the Copilot Client using the GitHub Copilot SDK.
// It starts a fresh Copilot CLI-backed session for each task run, auto-approves
// permission requests, and forwards raw events without mutation.
type GitHub struct {
	logger    *zap.Logger
	newClient func(opts *githubcopilot.ClientOptions) copilotClient
}

// NewGitHub constructs a GitHub Copilot client. Logger defaults to zap.NewNop.
func NewGitHub(logger *zap.Logger) *GitHub {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &GitHub{
		logger: logger,
		newClient: func(opts *githubcopilot.ClientOptions) copilotClient {
			return &sdkClient{inner: githubcopilot.NewClient(opts)}
		},
	}
}

// StartSession creates a Copilot session for the provided prompt and repository
// path. A buffered channel of RawEvent values is returned alongside a stop
// function that tears down the SDK resources.
func (g *GitHub) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan RawEvent, func(), error) {
	token, tokenKey := selectToken()
	if token == "" {
		err := fmt.Errorf("missing GitHub Copilot token (checked COPILOT_GITHUB_TOKEN, GH_TOKEN, GITHUB_TOKEN)")
		g.logger.Error("copilot token missing", zap.Error(err))
		return "", nil, nil, err
	}

	opts := &githubcopilot.ClientOptions{
		GithubToken:     token,
		Cwd:             repoPath,
		UseLoggedInUser: githubcopilot.Bool(false),
	}

	client := g.newClient(opts)
	if err := client.Start(ctx); err != nil {
		g.logger.Error("start copilot client", zap.String("env_key", tokenKey), zap.Error(err))
		return "", nil, nil, fmt.Errorf("start copilot client: %w", err)
	}

	session, err := client.CreateSession(ctx, &githubcopilot.SessionConfig{
		OnPermissionRequest: approvePermission,
		WorkingDirectory:    repoPath,
		Streaming:           true,
	})
	if err != nil {
		_ = client.Stop()
		g.logger.Error("create copilot session", zap.Error(err))
		return "", nil, nil, fmt.Errorf("create copilot session: %w", err)
	}

	events := make(chan RawEvent, eventBufferSize)
	handlerCtx, cancel := context.WithCancel(context.Background())

	unsubscribe := session.On(func(event githubcopilot.SessionEvent) {
		raw, err := encodeRawEvent(event)
		if err != nil {
			g.logger.Warn("marshal copilot event", zap.String("session_id", session.ID()), zap.Error(err))
			return
		}

		select {
		case <-handlerCtx.Done():
			return
		default:
		}

		select {
		case events <- raw:
		case <-handlerCtx.Done():
		case <-ctx.Done():
			g.logger.Warn("copilot session context cancelled while forwarding event", zap.String("session_id", session.ID()))
		default:
			g.logger.Warn("copilot events channel full; dropping event", zap.String("session_id", session.ID()))
		}
	})

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			unsubscribe()

			if err := client.Stop(); err != nil {
				g.logger.Error("stop copilot client", zap.Error(err))
			}

			close(events)
		})
	}

	if _, err := session.Send(ctx, githubcopilot.MessageOptions{Prompt: prompt}); err != nil {
		stop()
		g.logger.Error("send copilot prompt", zap.String("session_id", session.ID()), zap.Error(err))
		return "", nil, nil, fmt.Errorf("send prompt: %w", err)
	}

	g.logger.Info("copilot session started", zap.String("session_id", session.ID()), zap.String("repo_path", repoPath), zap.String("token_env", tokenKey))
	return session.ID(), events, stop, nil
}

func approvePermission(_ githubcopilot.PermissionRequest, _ githubcopilot.PermissionInvocation) (githubcopilot.PermissionRequestResult, error) {
	return githubcopilot.PermissionRequestResult{Kind: "approved"}, nil
}

func selectToken() (string, string) {
	for _, key := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if val, ok := os.LookupEnv(key); ok {
			val = strings.TrimSpace(val)
			if val != "" {
				return val, key
			}
		}
	}
	return "", ""
}

func encodeRawEvent(event githubcopilot.SessionEvent) (RawEvent, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var raw RawEvent
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}

	return raw, nil
}

type copilotClient interface {
	Start(ctx context.Context) error
	Stop() error
	CreateSession(ctx context.Context, config *githubcopilot.SessionConfig) (copilotSession, error)
}

type copilotSession interface {
	ID() string
	On(handler githubcopilot.SessionEventHandler) func()
	Send(ctx context.Context, options githubcopilot.MessageOptions) (string, error)
	Destroy() error
}

type sdkClient struct {
	inner *githubcopilot.Client
}

func (c *sdkClient) Start(ctx context.Context) error {
	return c.inner.Start(ctx)
}

func (c *sdkClient) Stop() error {
	return c.inner.Stop()
}

func (c *sdkClient) CreateSession(ctx context.Context, config *githubcopilot.SessionConfig) (copilotSession, error) {
	session, err := c.inner.CreateSession(ctx, config)
	if err != nil {
		return nil, err
	}

	return &sdkSession{inner: session}, nil
}

type sdkSession struct {
	inner *githubcopilot.Session
}

func (s *sdkSession) ID() string {
	return s.inner.SessionID
}

func (s *sdkSession) On(handler githubcopilot.SessionEventHandler) func() {
	return s.inner.On(handler)
}

func (s *sdkSession) Send(ctx context.Context, options githubcopilot.MessageOptions) (string, error) {
	return s.inner.Send(ctx, options)
}

func (s *sdkSession) Destroy() error {
	return s.inner.Destroy()
}
