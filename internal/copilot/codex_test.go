package copilot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestNewCodexDefaults(t *testing.T) {
	client := NewCodex(nil)

	require.NotNil(t, client.logger)
	require.NotNil(t, client.newCommand)
	require.Equal(t, codexScannerBufferSize, client.scannerBufferSize)
}

func TestNewCodexKeepsProvidedLogger(t *testing.T) {
	logger := zaptest.NewLogger(t)

	client := NewCodex(logger)

	require.Same(t, logger, client.logger)
	require.NotNil(t, client.newCommand)
}

func TestCodexPathResolutionUsesEnvOverrideFirst(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))

	overridePath := filepath.Join(t.TempDir(), "codex-wrapper")
	writeExecutableFile(t, overridePath, 0o755)
	t.Setenv(codexWrapperPathEnvVar, overridePath)

	workspaceDir := t.TempDir()
	setWorkingDirectory(t, workspaceDir)
	defaultPath := filepath.Join(workspaceDir, codexWrapperDefaultRelativePath)
	writeExecutableFile(t, defaultPath, 0o755)

	resolvedPath, err := client.resolveWrapperPath()
	require.NoError(t, err)
	require.Equal(t, overridePath, resolvedPath)
}

func TestCodexPathResolutionFallsBackToWorkingDirectoryDefault(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	t.Setenv(codexWrapperPathEnvVar, "")

	workspaceDir := t.TempDir()
	setWorkingDirectory(t, workspaceDir)
	defaultPath := filepath.Join(workspaceDir, codexWrapperDefaultRelativePath)
	writeExecutableFile(t, defaultPath, 0o755)

	resolvedPath, err := client.resolveWrapperPath()
	require.NoError(t, err)
	require.Equal(t, defaultPath, resolvedPath)
}

func TestCodexPathResolutionIgnoresTaskRepoPathForDefault(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	t.Setenv(codexWrapperPathEnvVar, "")
	workspaceDir := t.TempDir()
	taskRepoPath := t.TempDir()
	setWorkingDirectory(t, workspaceDir)

	defaultPath := filepath.Join(workspaceDir, codexWrapperDefaultRelativePath)
	writeExecutableFile(t, defaultPath, 0o755)

	resolvedPath, err := client.resolveWrapperPath()
	require.NoError(t, err)
	require.Equal(t, defaultPath, resolvedPath)

	_, events, stop, err := client.StartSession(context.Background(), "hello", taskRepoPath)
	require.NoError(t, err)
	idle := readEventWithTimeout(t, events)
	require.Equal(t, codexIdleEventType, idle["type"])
	waitForChannelClosed(t, events)
	stop()
}

func TestCodexPathResolutionMissingWrapperReturnsActionableError(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	t.Setenv(codexWrapperPathEnvVar, "")
	setWorkingDirectory(t, t.TempDir())

	_, err := client.resolveWrapperPath()
	require.Error(t, err)
	require.ErrorContains(t, err, "codex wrapper binary not found")
	require.ErrorContains(t, err, codexWrapperDefaultRelativePath)
	require.ErrorContains(t, err, codexWrapperPathEnvVar)
}

func TestCodexPathResolutionRejectsNonExecutableFile(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	workspaceDir := t.TempDir()
	setWorkingDirectory(t, workspaceDir)

	defaultPath := filepath.Join(workspaceDir, codexWrapperDefaultRelativePath)
	writeExecutableFile(t, defaultPath, 0o644)
	t.Setenv(codexWrapperPathEnvVar, "")

	_, err := client.resolveWrapperPath()
	require.Error(t, err)
	require.ErrorContains(t, err, "not executable")
	require.ErrorContains(t, err, defaultPath)
}

func TestCodexStartSessionStreamsRawEventsInOrder(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-stream.sh")
	writeExecutableFile(t, wrapperPath, 0o755)
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
if [ "$1" != "--repo-path" ]; then
  echo "expected --repo-path" >&2
  exit 11
fi
if [ "$3" != "--prompt" ]; then
  echo "expected --prompt" >&2
  exit 12
fi
printf '{"type":"event.one","repo":"%s","prompt":"%s"}\n' "$2" "$4"
echo "wrapper stderr line" >&2
printf '{"type":"event.two","n":2}\n'
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	sessionID, events, stop, err := client.StartSession(context.Background(), "hello from prompt", repoPath)
	require.NoError(t, err)
	require.NotEmpty(t, sessionID)
	require.NotNil(t, events)
	require.NotNil(t, stop)

	first := readEventWithTimeout(t, events)
	require.Equal(t, "event.one", first["type"])
	require.Equal(t, repoPath, first["repo"])
	require.Equal(t, "hello from prompt", first["prompt"])

	second := readEventWithTimeout(t, events)
	require.Equal(t, "event.two", second["type"])
	require.EqualValues(t, float64(2), second["n"])

	idle := readEventWithTimeout(t, events)
	require.Equal(t, "session.idle", idle["type"])

	waitForChannelClosed(t, events)
	stop()
}

func TestCodexStartSessionStopIsIdempotent(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-block.sh")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
while true; do
  sleep 1
done
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	_, events, stop, err := client.StartSession(context.Background(), "hello", repoPath)
	require.NoError(t, err)
	require.NotNil(t, stop)

	stop()
	stop()

	waitForChannelClosed(t, events)
}

func TestCodexIdleNaturalExitEmitsSingleIdleThenCloses(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-natural-exit.sh")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
printf '{"type":"event.one"}\n'
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	_, events, stop, err := client.StartSession(context.Background(), "hello", repoPath)
	require.NoError(t, err)

	first := readEventWithTimeout(t, events)
	require.Equal(t, "event.one", first["type"])

	idle := readEventWithTimeout(t, events)
	require.Equal(t, "session.idle", idle["type"])

	waitForChannelClosed(t, events)
	stop()
}

func TestCodexIdleContextCancellationClosesStream(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-context-cancel.sh")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
sleep 10
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	ctx, cancel := context.WithCancel(context.Background())
	_, events, stop, err := client.StartSession(ctx, "hello", repoPath)
	require.NoError(t, err)

	cancel()
	waitForChannelClosed(t, events)

	stop()
	stop()
}

func TestCodexMalformedLineSkipsAndContinues(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-malformed.sh")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
printf '{"type":"event.before"}\n'
printf '{not-json}\n'
printf '{"type":"event.after"}\n'
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	_, events, stop, err := client.StartSession(context.Background(), "hello", repoPath)
	require.NoError(t, err)

	before := readEventWithTimeout(t, events)
	require.Equal(t, "event.before", before["type"])

	after := readEventWithTimeout(t, events)
	require.Equal(t, "event.after", after["type"])

	idle := readEventWithTimeout(t, events)
	require.Equal(t, codexIdleEventType, idle["type"])

	waitForChannelClosed(t, events)
	stop()
}

func TestCodexMalformedScannerErrorStillEmitsIdle(t *testing.T) {
	client := NewCodex(zaptest.NewLogger(t))
	client.scannerBufferSize = 128
	repoPath := t.TempDir()
	wrapperPath := filepath.Join(t.TempDir(), "codex-wrapper-scan-err.sh")
	require.NoError(t, os.WriteFile(wrapperPath, []byte(`#!/bin/sh
printf '{"type":"event.before"}\n'
long=$(awk 'BEGIN { for (i = 0; i < 512; i++) printf "a" }')
printf '{"type":"%s"}\n' "$long"
`), 0o755))
	t.Setenv(codexWrapperPathEnvVar, wrapperPath)

	_, events, stop, err := client.StartSession(context.Background(), "hello", repoPath)
	require.NoError(t, err)

	before := readEventWithTimeout(t, events)
	require.Equal(t, "event.before", before["type"])

	idle := readEventWithTimeout(t, events)
	require.Equal(t, codexIdleEventType, idle["type"])

	waitForChannelClosed(t, events)
	stop()
}

func readEventWithTimeout(t *testing.T, events <-chan RawEvent) RawEvent {
	t.Helper()

	select {
	case event, ok := <-events:
		require.True(t, ok, "expected event before channel closed")
		return event
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timed out waiting for event")
		return nil
	}
}

func waitForChannelClosed(t *testing.T, events <-chan RawEvent) {
	t.Helper()

	select {
	case _, ok := <-events:
		if ok {
			require.FailNow(t, "expected channel to be closed")
		}
	case <-time.After(2 * time.Second):
		require.FailNow(t, fmt.Sprintf("timed out waiting for events channel to close within %s", 2*time.Second))
	}
}

func writeExecutableFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), mode))
}

func setWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}
