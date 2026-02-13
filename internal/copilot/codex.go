package copilot

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
)

var _ Client = (*Codex)(nil)

const (
	codexWrapperPathEnvVar          = "YARALPHO_CODEX_WRAPPER_PATH"
	codexWrapperDefaultRelativePath = "internal/copilot/codex-ts/bin/codex-wrapper-linux-x64"
	codexScannerBufferSize          = 8 * 1024 * 1024
	codexIdleEventType              = "session.idle"
	codexLogPreviewLimitBytes       = 256
)

// Codex implements the Copilot Client contract for the Codex provider.
type Codex struct {
	logger            *zap.Logger
	newCommand        func(ctx context.Context, name string, args ...string) *exec.Cmd
	lookupEnv         func(key string) (string, bool)
	stat              func(name string) (os.FileInfo, error)
	getwd             func() (string, error)
	scannerBufferSize int
}

// NewCodex constructs a Codex client. Logger defaults to zap.NewNop.
func NewCodex(logger *zap.Logger) *Codex {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Codex{
		logger:            logger,
		newCommand:        exec.CommandContext,
		lookupEnv:         os.LookupEnv,
		stat:              os.Stat,
		getwd:             os.Getwd,
		scannerBufferSize: codexScannerBufferSize,
	}
}

func (c *Codex) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan RawEvent, func(), error) {
	wrapperPath, err := c.resolveWrapperPath()
	if err != nil {
		return "", nil, nil, err
	}

	newCommand := c.newCommand
	if newCommand == nil {
		newCommand = exec.CommandContext
	}

	cmd := newCommand(ctx, wrapperPath, "--repo-path", repoPath, "--prompt", prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, nil, fmt.Errorf("codex wrapper stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, nil, fmt.Errorf("codex wrapper stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", nil, nil, fmt.Errorf("start codex wrapper: %w", err)
	}

	sessionID := fmt.Sprintf("codex-%d", cmd.Process.Pid)
	events := make(chan RawEvent, eventBufferSize)

	var closeOnce sync.Once
	closeEvents := func() {
		closeOnce.Do(func() {
			close(events)
		})
	}

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			if cmd.Process == nil {
				return
			}
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				c.logger.Warn("stop codex wrapper process", zap.String("session_id", sessionID), zap.Error(err))
			}
		})
	}

	go func() {
		scannerBufferSize := c.scannerBufferSize
		if scannerBufferSize <= 0 {
			scannerBufferSize = codexScannerBufferSize
		}

		var stderrWG sync.WaitGroup
		stderrWG.Add(1)
		go func() {
			defer stderrWG.Done()
			stderrScanner := bufio.NewScanner(stderr)
			stderrScanner.Buffer(make([]byte, 0, scannerInitialCapacity(scannerBufferSize)), scannerBufferSize)
			for stderrScanner.Scan() {
				c.logger.Warn("codex wrapper stderr", zap.String("session_id", sessionID), zap.String("line", stderrScanner.Text()))
			}
			if err := stderrScanner.Err(); err != nil && !isPipeClosedError(err) {
				c.logger.Warn("codex wrapper stderr scan error", zap.String("session_id", sessionID), zap.Error(err))
			}
		}()

		stdoutScanner := bufio.NewScanner(stdout)
		stdoutScanner.Buffer(make([]byte, 0, scannerInitialCapacity(scannerBufferSize)), scannerBufferSize)
		for stdoutScanner.Scan() {
			line := stdoutScanner.Bytes()

			var event RawEvent
			if err := json.Unmarshal(line, &event); err != nil {
				c.logger.Warn(
					"skip malformed codex wrapper stdout event",
					zap.String("session_id", sessionID),
					zap.Int("line_bytes", len(line)),
					zap.String("line_preview", previewForLog(line, codexLogPreviewLimitBytes)),
					zap.Error(err),
				)
				continue
			}

			select {
			case events <- event:
			case <-ctx.Done():
				c.logger.Warn("context canceled while forwarding codex event", zap.String("session_id", sessionID))
				stop()
				stderrWG.Wait()
				closeEvents()
				return
			}
		}

		scanErr := stdoutScanner.Err()
		if scanErr != nil && !isPipeClosedError(scanErr) {
			c.logger.Error("codex wrapper stdout scan error", zap.String("session_id", sessionID), zap.Error(scanErr))
		}

		waitErr := cmd.Wait()
		if waitErr != nil {
			c.logger.Warn("codex wrapper process exited with error", zap.String("session_id", sessionID), zap.Error(waitErr))
		}

		stderrWG.Wait()

		shouldEmitIdle := waitErr == nil || (scanErr != nil && !isPipeClosedError(scanErr))
		if shouldEmitIdle {
			select {
			case events <- RawEvent{"type": codexIdleEventType}:
			case <-ctx.Done():
				c.logger.Warn("context canceled before synthetic codex idle event", zap.String("session_id", sessionID))
			}
		}

		closeEvents()
	}()

	c.logger.Info("codex wrapper session started", zap.String("session_id", sessionID), zap.String("repo_path", repoPath), zap.String("wrapper_path", wrapperPath))
	return sessionID, events, stop, nil
}

func previewForLog(raw []byte, limit int) string {
	if limit <= 0 || len(raw) <= limit {
		return string(raw)
	}
	return string(raw[:limit]) + "...(truncated)"
}

func scannerInitialCapacity(maxSize int) int {
	const defaultInitialCap = 64 * 1024
	if maxSize <= 0 {
		return defaultInitialCap
	}
	if maxSize < defaultInitialCap {
		return maxSize
	}
	return defaultInitialCap
}

func isPipeClosedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrClosed) {
		return true
	}
	return strings.Contains(err.Error(), "file already closed")
}

func (c *Codex) resolveWrapperPath() (string, error) {
	candidatePath, source := c.wrapperPathCandidate()

	info, err := c.stat(candidatePath)
	if err != nil {
		resolutionErr := fmt.Errorf("codex wrapper binary not found at %q (%s): set %s or build %s: %w",
			candidatePath,
			source,
			codexWrapperPathEnvVar,
			codexWrapperDefaultRelativePath,
			err,
		)
		c.logger.Error("resolve codex wrapper path", zap.String("candidate_path", candidatePath), zap.String("source", source), zap.Error(resolutionErr))
		return "", resolutionErr
	}

	if info.IsDir() {
		err := fmt.Errorf("codex wrapper path %q (%s) is a directory", candidatePath, source)
		c.logger.Error("resolve codex wrapper path", zap.String("candidate_path", candidatePath), zap.String("source", source), zap.Error(err))
		return "", err
	}

	if info.Mode().Perm()&0o111 == 0 {
		err := fmt.Errorf("codex wrapper path %q (%s) is not executable (mode %04o)", candidatePath, source, info.Mode().Perm())
		c.logger.Error("resolve codex wrapper path", zap.String("candidate_path", candidatePath), zap.String("source", source), zap.Error(err))
		return "", err
	}

	c.logger.Info("resolved codex wrapper path", zap.String("wrapper_path", candidatePath), zap.String("source", source))
	return candidatePath, nil
}

func (c *Codex) wrapperPathCandidate() (string, string) {
	lookup := c.lookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	if envPath, ok := lookup(codexWrapperPathEnvVar); ok && strings.TrimSpace(envPath) != "" {
		return envPath, codexWrapperPathEnvVar
	}

	getwd := c.getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	wd, err := getwd()
	if err != nil || strings.TrimSpace(wd) == "" {
		return codexWrapperDefaultRelativePath, "default"
	}

	return filepath.Join(wd, codexWrapperDefaultRelativePath), "default"
}
