package impact_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/docscheck/impact"
)

// The binary itself, in a repository of the test's own making, so the test
// holds in an export of this tree that has no .git.
func TestGitAnswersOnlyForItsOwnWorkingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	top := t.TempDir()
	// #nosec G204 -- the directory is the test's own.
	if out, err := exec.CommandContext(ctx, "git", "init", "-q", top).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	git := impact.Repository(impact.Git(top, os.Environ()), top)
	out, err := git(ctx, "rev-parse", "--is-shallow-repository")
	if err != nil || strings.TrimSpace(string(out)) != "false" {
		t.Fatalf("at the top level: %q, %v", out, err)
	}

	inside := filepath.Join(top, "docs")
	if err := os.Mkdir(inside, 0o750); err != nil {
		t.Fatal(err)
	}
	git = impact.Repository(impact.Git(inside, os.Environ()), inside)
	if _, err := git(ctx, "rev-parse", "--is-shallow-repository"); !errors.Is(err, impact.ErrNotMeasured) {
		t.Errorf("below the top level: %v, want NOT MEASURED", err)
	}

	environ := append(os.Environ(), "GIT_DIR="+filepath.Join(top, ".git"))
	outside := t.TempDir()
	git = impact.Repository(impact.Git(outside, environ), outside)
	if _, err := git(ctx, "rev-parse", "--is-shallow-repository"); !errors.Is(err, impact.ErrNotMeasured) {
		t.Errorf("GIT_DIR pointing elsewhere was followed: %v", err)
	}
}
