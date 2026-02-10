package tracker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

type stubConfig map[string]string

func (c stubConfig) Get(key string) (string, error) {
	val, ok := c[key]
	if !ok {
		return "", errors.New("not found")
	}
	return val, nil
}

func TestBeadsNewUsesRepoFromConfig(t *testing.T) {
	repoDir := t.TempDir()
	cfg := stubConfig{"YARALPHO_BD_REPO": repoDir}

	b, err := NewBeads(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.repoPath != repoDir {
		t.Fatalf("repoPath = %q, want %q", b.repoPath, repoDir)
	}
}

func TestBeadsNewErrorsWhenRepoDirMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	cfg := stubConfig{"YARALPHO_BD_REPO": missing}

	if _, err := NewBeads(cfg, nil); err == nil {
		t.Fatalf("expected error for missing repo directory")
	}
}

func TestBeadsNewMissingRepo(t *testing.T) {
	_, err := NewBeads(stubConfig{}, nil)
	if err == nil {
		t.Fatalf("expected error for missing repo path")
	}
}

func TestBeadsListChildrenParsesOrder(t *testing.T) {
	output := "Header line\n  ↳ ● P2] [task] yaralpho-62m.13: Task 9\n  ↳ ○ yaralpho-62m.14: Task 10\n"
	runner := &fakeRunner{output: []byte(output)}
	b := &Beads{
		repoPath: "/bd/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	children, err := b.ListChildren(context.Background(), "yaralpho-62m")
	if err != nil {
		t.Fatalf("ListChildren error: %v", err)
	}
	want := []string{"yaralpho-62m.13", "yaralpho-62m.14"}
	if len(children) != len(want) {
		t.Fatalf("got %d children, want %d", len(children), len(want))
	}
	for i := range want {
		if children[i] != want[i] {
			t.Fatalf("child %d = %q, want %q", i, children[i], want[i])
		}
	}

	if runner.dir != "/bd/repo" {
		t.Fatalf("command dir = %q, want /bd/repo", runner.dir)
	}
	if got, wantArg := runner.args, []string{"bd", "show", "yaralpho-62m"}; !equalStrings(got, wantArg) {
		t.Fatalf("args = %#v, want %#v", got, wantArg)
	}
}

func TestBeadsIsEpicTrueWhenChildrenExist(t *testing.T) {
	runner := &fakeRunner{output: []byte("  ↳ child-1\n")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	isEpic, err := b.IsEpic(context.Background(), "abc")
	if err != nil {
		t.Fatalf("IsEpic error: %v", err)
	}
	if !isEpic {
		t.Fatalf("expected epic to be true")
	}
}

func TestBeadsIsEpicFalseWhenNoChildren(t *testing.T) {
	runner := &fakeRunner{output: []byte("no children here")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	isEpic, err := b.IsEpic(context.Background(), "abc")
	if err != nil {
		t.Fatalf("IsEpic error: %v", err)
	}
	if isEpic {
		t.Fatalf("expected epic to be false")
	}
}

func TestBeadsListChildrenPropagatesError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	if _, err := b.ListChildren(context.Background(), "abc"); err == nil {
		t.Fatalf("expected error from runner")
	}
}

func TestBeadsListChildrenHonorsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner := &fakeRunner{output: []byte("  ↳ child-1")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	if _, err := b.ListChildren(ctx, "abc"); err == nil {
		t.Fatalf("expected context error")
	}
}

func TestBeadsAddCommentInline(t *testing.T) {
	runner := &fakeRunner{}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	if err := b.AddComment(context.Background(), "ref-1", "hello world"); err != nil {
		t.Fatalf("AddComment error: %v", err)
	}

	wantArgs := []string{"bd", "comments", "add", "ref-1", "hello world"}
	if !equalStrings(runner.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}
	if runner.dir != "/repo" {
		t.Fatalf("dir = %q, want /repo", runner.dir)
	}
}

func TestBeadsAddCommentWithTempFile(t *testing.T) {
	var (
		capturedArgs []string
		capturedDir  string
		body         string
	)

	run := func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		capturedDir = dir
		capturedArgs = args
		if len(args) != 6 {
			return nil, errors.New("unexpected arg count")
		}
		content, err := os.ReadFile(args[5])
		if err != nil {
			return nil, err
		}
		body = string(content)
		return nil, nil
	}

	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      run,
	}

	comment := "hello\nworld"
	if err := b.AddComment(context.Background(), "ref-2", comment); err != nil {
		t.Fatalf("AddComment error: %v", err)
	}

	wantPrefix := []string{"bd", "comments", "add", "ref-2", "-f"}
	for i := range wantPrefix {
		if capturedArgs[i] != wantPrefix[i] {
			t.Fatalf("arg %d = %q, want %q", i, capturedArgs[i], wantPrefix[i])
		}
	}
	if capturedDir != "/repo" {
		t.Fatalf("dir = %q, want /repo", capturedDir)
	}
	if body != comment {
		t.Fatalf("comment body = %q, want %q", body, comment)
	}
	if _, err := os.Stat(capturedArgs[5]); !os.IsNotExist(err) {
		t.Fatalf("temp file was not removed")
	}
}

func TestBeadsAddCommentPropagatesRunnerError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	if err := b.AddComment(context.Background(), "ref-3", "text"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBeadsFetchCommentsParsesOutput(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[
			{
				"id": "ref-1",
				"comments": [
					{
						"id": 1,
						"author": "Alice",
						"text": "hello",
						"created_at": "2026-02-09T12:46:10Z",
						"updated_at": "2026-02-10T12:46:10Z"
					},
					{
						"id": 2,
						"author": "Bob",
						"text": "second",
						"created_at": "2026-02-11T12:46:10Z"
					}
				]
			}
		]`),
	}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	comments, err := b.FetchComments(context.Background(), "ref-1")
	if err != nil {
		t.Fatalf("FetchComments error: %v", err)
	}
	if runner.dir != "/repo" {
		t.Fatalf("dir = %q, want /repo", runner.dir)
	}
	wantArgs := []string{"bd", "view", "ref-1", "--json"}
	if !equalStrings(runner.args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}
	if len(comments) != 2 {
		t.Fatalf("comments len = %d, want 2", len(comments))
	}

	if comments[0].ID != "1" || comments[0].Author != "Alice" || comments[0].Text != "hello" {
		t.Fatalf("first comment = %#v", comments[0])
	}
	if !comments[0].CreatedAt.Equal(time.Date(2026, 2, 9, 12, 46, 10, 0, time.UTC)) {
		t.Fatalf("created_at = %v", comments[0].CreatedAt)
	}
	if !comments[0].UpdatedAt.Equal(time.Date(2026, 2, 10, 12, 46, 10, 0, time.UTC)) {
		t.Fatalf("updated_at = %v", comments[0].UpdatedAt)
	}
	if comments[1].ID != "2" || comments[1].Author != "Bob" || comments[1].Text != "second" {
		t.Fatalf("second comment = %#v", comments[1])
	}
	if !comments[1].CreatedAt.Equal(time.Date(2026, 2, 11, 12, 46, 10, 0, time.UTC)) {
		t.Fatalf("second created_at = %v", comments[1].CreatedAt)
	}
	if !comments[1].UpdatedAt.IsZero() {
		t.Fatalf("expected zero updated_at, got %v", comments[1].UpdatedAt)
	}
}

func TestBeadsFetchCommentsMissingComments(t *testing.T) {
	runner := &fakeRunner{
		output: []byte(`[{"id":"ref-1","title":"noop"}]`),
	}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	comments, err := b.FetchComments(context.Background(), "ref-1")
	if err != nil {
		t.Fatalf("FetchComments error: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments len = %d, want 0", len(comments))
	}
}

func TestBeadsFetchCommentsPropagatesRunnerError(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	b := &Beads{
		repoPath: "/repo",
		logger:   zap.NewNop(),
		timeout:  time.Second,
		run:      runner.run,
	}

	if _, err := b.FetchComments(context.Background(), "ref-1"); err == nil {
		t.Fatalf("expected error")
	}
}

type fakeRunner struct {
	output []byte
	err    error
	dir    string
	args   []string
}

func (f *fakeRunner) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	f.dir = dir
	f.args = args
	return f.output, f.err
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
