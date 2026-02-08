package tracker

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"go.uber.org/zap"
)

const defaultBeadsTimeout = 5 * time.Second

// commandRunner abstracts exec.CommandContext for testability.
type commandRunner func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Beads implements Tracker by invoking the beads CLI (`bd show <ref>`) inside
// the configured repository path.
type Beads struct {
	repoPath string
	logger   *zap.Logger
	timeout  time.Duration
	run      commandRunner
}

// NewBeads constructs a Beads tracker using the configured beads repository
// path. Logger defaults to zap.NewNop and timeout defaults to five seconds.
func NewBeads(cfg config.Config, logger *zap.Logger) (*Beads, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	repoPath, err := cfg.Get(config.BdRepoKey)
	if err != nil {
		return nil, fmt.Errorf("bd repo: %w", err)
	}
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return nil, fmt.Errorf("bd repo is empty")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Beads{
		repoPath: repoPath,
		logger:   logger,
		timeout:  defaultBeadsTimeout,
		run:      defaultCommandRunner,
	}, nil
}

// IsEpic returns true when the reference has children. Non-epics return false
// without error.
func (b *Beads) IsEpic(ctx context.Context, ref string) (bool, error) {
	children, err := b.listChildren(ctx, ref)
	if err != nil {
		return false, err
	}

	isEpic := len(children) > 0
	b.logger.Debug("beads is-epic", zap.String("ref", ref), zap.Bool("is_epic", isEpic), zap.Int("child_count", len(children)))
	return isEpic, nil
}

// ListChildren returns ordered child references for the provided epic.
func (b *Beads) ListChildren(ctx context.Context, ref string) ([]string, error) {
	children, err := b.listChildren(ctx, ref)
	if err != nil {
		return nil, err
	}

	b.logger.Debug("beads children", zap.String("ref", ref), zap.Int("child_count", len(children)))
	return children, nil
}

func (b *Beads) listChildren(ctx context.Context, ref string) ([]string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("ref is required")
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	output, err := b.run(ctx, b.repoPath, "bd", "show", ref)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		b.logger.Error("bd show failed", zap.String("ref", ref), zap.Error(err))
		return nil, fmt.Errorf("bd show %s: %w", ref, err)
	}

	return parseChildren(string(output)), nil
}

func defaultCommandRunner(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command provided")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir

	return cmd.CombinedOutput()
}

func parseChildren(output string) []string {
	lines := strings.Split(output, "\n")
	children := make([]string, 0)
	for _, line := range lines {
		arrowIdx := strings.Index(line, "↳")
		if arrowIdx == -1 {
			continue
		}

		rest := strings.TrimSpace(line[arrowIdx+len("↳"):])
		if rest == "" {
			continue
		}

		ref := extractRef(rest)
		if ref != "" {
			children = append(children, ref)
		}
	}
	return children
}

func extractRef(text string) string {
	// bd show lines typically look like: "  ↳ ○ yaralpho-62m.13: Task 9 ..."
	fields := strings.Fields(text)
	for _, f := range fields {
		f = strings.TrimSpace(strings.TrimSuffix(f, ":"))
		if refPattern.MatchString(f) {
			return f
		}
	}
	return ""
}

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
