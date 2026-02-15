package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	logger  *zap.Logger
	timeout time.Duration
	run     commandRunner
}

// NewBeads constructs a Beads tracker using the configured beads repository
// path. Logger defaults to zap.NewNop and timeout defaults to five seconds.
func NewBeads(cfg config.Config, logger *zap.Logger) (*Beads, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Beads{
		logger:  logger,
		timeout: defaultBeadsTimeout,
		run:     defaultCommandRunner,
	}, nil
}

func defaultCommandRunner(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no command provided")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir

	return cmd.CombinedOutput()
}

func (b *Beads) normalizeRepoPath(repoPath string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", fmt.Errorf("repo path is required")
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("repo path abs: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("repo path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo path is not a directory: %s", absPath)
	}

	return absPath, nil
}

// AddComment is a placeholder implementation that will be wired to bd
// comments in subsequent tasks.
func (b *Beads) AddComment(ctx context.Context, repoPath string, ref string, text string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("ref is required")
	}

	repoPath, err := b.normalizeRepoPath(repoPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return err
	}

	args := []string{"bd", "comments", "add", ref}
	useTempFile := strings.ContainsAny(text, "\n\r")

	if useTempFile {
		tempFile, err := os.CreateTemp("", "bd-comment-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		defer os.Remove(tempFile.Name())

		if _, err := tempFile.WriteString(text); err != nil {
			tempFile.Close()
			return fmt.Errorf("write temp file: %w", err)
		}
		if err := tempFile.Close(); err != nil {
			return fmt.Errorf("close temp file: %w", err)
		}

		args = append(args, "-f", tempFile.Name())
	} else {
		args = append(args, text)
	}

	if _, err := b.run(ctx, repoPath, args...); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		b.logger.Error("bd comments add failed", zap.String("ref", ref), zap.String("repo_path", repoPath), zap.Error(err))
		return fmt.Errorf("bd comments add %s: %w", ref, err)
	}

	return nil
}

// FetchComments currently returns no comments; bd wiring will be added later.
func (b *Beads) FetchComments(ctx context.Context, repoPath string, ref string) ([]Comment, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("ref is required")
	}

	repoPath, err := b.normalizeRepoPath(repoPath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	output, err := b.run(ctx, repoPath, "bd", "view", ref, "--json")
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		b.logger.Error("bd view failed", zap.String("ref", ref), zap.String("repo_path", repoPath), zap.Error(err))
		return nil, fmt.Errorf("bd view %s: %w", ref, err)
	}

	var issues []struct {
		ID       string `json:"id"`
		Comments []struct {
			ID        json.Number `json:"id"`
			Author    string      `json:"author"`
			Text      string      `json:"text"`
			CreatedAt string      `json:"created_at"`
			UpdatedAt string      `json:"updated_at"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		b.logger.Error("parse bd view failed", zap.String("ref", ref), zap.String("repo_path", repoPath), zap.Error(err))
		return nil, fmt.Errorf("parse bd view %s: %w", ref, err)
	}
	if len(issues) == 0 {
		return []Comment{}, nil
	}

	var issueComments []struct {
		ID        json.Number `json:"id"`
		Author    string      `json:"author"`
		Text      string      `json:"text"`
		CreatedAt string      `json:"created_at"`
		UpdatedAt string      `json:"updated_at"`
	}
	for i := range issues {
		if issues[i].ID == ref {
			issueComments = issues[i].Comments
			break
		}
	}
	if issueComments == nil && len(issues) > 0 {
		issueComments = issues[0].Comments
	}

	comments := make([]Comment, 0, len(issueComments))
	for _, c := range issueComments {
		var created time.Time
		if c.CreatedAt != "" {
			created, err = time.Parse(time.RFC3339, c.CreatedAt)
			if err != nil {
				return nil, fmt.Errorf("parse created_at for %s: %w", ref, err)
			}
		}

		var updated time.Time
		if c.UpdatedAt != "" {
			updated, err = time.Parse(time.RFC3339, c.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("parse updated_at for %s: %w", ref, err)
			}
		}

		comments = append(comments, Comment{
			ID:        c.ID.String(),
			Author:    c.Author,
			Text:      c.Text,
			CreatedAt: created,
			UpdatedAt: updated,
		})
	}

	return comments, nil
}

// GetTitle returns the issue title for the given reference.
func (b *Beads) GetTitle(ctx context.Context, repoPath string, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("ref is required")
	}

	repoPath, err := b.normalizeRepoPath(repoPath)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	output, err := b.run(ctx, repoPath, "bd", "view", ref, "--json")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		b.logger.Error("bd view failed", zap.String("ref", ref), zap.String("repo_path", repoPath), zap.Error(err))
		return "", fmt.Errorf("bd view %s: %w", ref, err)
	}

	var issues []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(output, &issues); err != nil {
		b.logger.Error("parse bd view failed", zap.String("ref", ref), zap.String("repo_path", repoPath), zap.Error(err))
		return "", fmt.Errorf("parse bd view %s: %w", ref, err)
	}
	if len(issues) == 0 {
		return "", nil
	}

	for i := range issues {
		if issues[i].ID == ref {
			return strings.TrimSpace(issues[i].Title), nil
		}
	}

	return strings.TrimSpace(issues[0].Title), nil
}
