package repofiles_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/guardana/playground/internal/docscheck/repofiles"
)

// withLister is a directory holding the repository's own lister and the
// named files, each holding its own name.
func withLister(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join("../../..", repofiles.Script))
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string][]byte{repofiles.Script: script}
	for _, name := range files {
		contents[name] = []byte(name + "\n")
	}
	for name, data := range contents {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o700); err != nil { // #nosec G306 -- the lister must be executable.
			t.Fatal(err)
		}
	}
	return root
}

func TestTrackedLeavesOutWhatGitDoesNotTrack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := withLister(t, "docs/tracked.md", "docs/untracked.md", "deleted.md")
	for _, args := range [][]string{{"init", "-q"}, {"add", "docs/tracked.md", "deleted.md"}} {
		cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- the test's own arguments.
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.Remove(filepath.Join(root, "deleted.md")); err != nil {
		t.Fatal(err)
	}
	got, err := repofiles.Tracked(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"docs/tracked.md"}) {
		t.Errorf("Tracked = %q, want only docs/tracked.md", got)
	}
}

// An export has no .git: what the lister finds there is the committed tree.
func TestTrackedInAnExportIsWhatTheListerFinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root := withLister(t, "docs/page.md")
	got, err := repofiles.Tracked(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"docs/page.md", repofiles.Script}) {
		t.Errorf("Tracked = %q, want docs/page.md and the lister", got)
	}
}
