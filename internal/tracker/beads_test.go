package tracker

import (
	"context"
	"errors"
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
	cfg := stubConfig{"YARALPHO_BD_REPO": "/tmp/repo"}

	b, err := NewBeads(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.repoPath != "/tmp/repo" {
		t.Fatalf("repoPath = %q, want %q", b.repoPath, "/tmp/repo")
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
