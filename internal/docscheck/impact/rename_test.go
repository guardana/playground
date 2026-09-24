package impact_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/guardana/playground/internal/docscheck/impact"
)

// hermetic keeps the user's and the system's git configuration out, so a
// diff.renames setting on one machine cannot decide the result.
func hermetic() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=lab", "GIT_AUTHOR_EMAIL=lab@example.invalid",
		"GIT_COMMITTER_NAME=lab", "GIT_COMMITTER_EMAIL=lab@example.invalid")
}

func gitIn(ctx context.Context, t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- the test's own arguments.
	cmd.Dir = dir
	cmd.Env = hermetic()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// renamedRepository commits a page covering runner/old.go, then a commit that
// only moves runner/old.go to runner/new.go.
func renamedRepository(ctx context.Context, t *testing.T) string {
	t.Helper()
	top := t.TempDir()
	gitIn(ctx, t, top, "init", "-q")
	for name, body := range map[string]string{
		"runner/old.go":   "package runner\n\n// Lab runs one scenario.\nfunc Lab() {}\n",
		"docs/runbook.md": "# Runbook\n",
	} {
		if err := os.MkdirAll(filepath.Join(top, filepath.Dir(name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(top, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(ctx, t, top, "add", "runner/old.go", "docs/runbook.md")
	gitIn(ctx, t, top, "commit", "-q", "-m", "first")
	gitIn(ctx, t, top, "mv", "runner/old.go", "runner/new.go")
	gitIn(ctx, t, top, "commit", "-q", "-m", "move the runner")
	return top
}

func TestARenameListsTheOldPathToo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	top := renamedRepository(ctx, t)
	git := impact.Repository(impact.Git(top, hermetic()), top)

	changed, err := impact.Changed(ctx, git, "HEAD~1..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(changed, "runner/old.go") || !slices.Contains(changed, "runner/new.go") {
		t.Errorf("Changed = %q, want runner/old.go and runner/new.go", changed)
	}

	rows, err := impact.Stale(ctx, git, []impact.Page{{Path: "docs/runbook.md", Covers: []string{"runner/old.go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Since) != 1 || rows[0].Since[0].Subject != "move the runner" {
		t.Errorf("Stale = %+v, want the move as the one commit since the page", rows)
	}
}
