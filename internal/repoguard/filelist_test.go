package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// copyScripts puts the file list and the guards into dir/scripts, the way a
// copy of the repository would hold them.
func copyScripts(t *testing.T, dir string) {
	t.Helper()
	scripts, err := filepath.Glob(filepath.Join(repoRoot, "scripts", "*.sh"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no scripts to copy: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, script := range scripts {
		body, err := os.ReadFile(script) // #nosec G304 -- a script of this repository.
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scripts", filepath.Base(script)), body, 0o700); err != nil { // #nosec G306 -- a script to run.
			t.Fatal(err)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...) // #nosec G204 -- fixed arguments.
	command.Dir = dir
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A copy inside another repository that ignores it is not that repository's
// work tree: the list is read from the files, not from a git that lists none.
func TestTheListOfACopyInsideAnotherRepositoryIsItsOwn(t *testing.T) {
	parent := t.TempDir()
	git(t, parent, "init", "-q")
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("copy/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(parent, "copy")
	copyScripts(t, copied)
	command := exec.Command("./scripts/repo-files.sh")
	command.Dir = copied
	out, err := command.Output()
	if err != nil || !strings.Contains(string(out), "scripts/repo-files.sh") {
		t.Fatalf("the copy listed %q: %v", out, err)
	}
}

// A work tree whose every file git ignores lists nothing, and a guard that
// scanned that list would report clean having read no file.
func TestGuardsFailWhenTheListIsEmpty(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	copyScripts(t, dir)
	for _, guard := range []string{"check-attribution.sh", "check-hygiene.sh", "check-file-sizes.sh"} {
		command := exec.Command("./scripts/" + guard) // #nosec G204 -- script names from the literal list.
		command.Dir = dir
		if err := command.Run(); err == nil {
			t.Errorf("%s passed on an empty file list", guard)
		}
	}
}

// A name holding a line break cannot be printed one per line, and a guard
// reading the list would judge two names that do not exist instead of it.
func TestANameHoldingALineBreakFailsTheList(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	copyScripts(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "probe\nname.md"), []byte("notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("./scripts/repo-files.sh")
	command.Dir = dir
	if out, err := command.Output(); err == nil {
		t.Errorf("the list took a name with a line break: %q", out)
	}
}

// git lists a repository nested in the tree as one directory and nothing in
// it, so a guard reading the list would never see its files.
func TestANestedRepositoryFailsTheList(t *testing.T) {
	for _, added := range []bool{false, true} {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		copyScripts(t, dir)
		nested := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatal(err)
		}
		git(t, nested, "init", "-q")
		if err := os.WriteFile(filepath.Join(nested, "notes.md"), []byte("notes\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git(t, nested, "add", "notes.md")
		git(t, nested, "-c", "user.name=probe", "-c", "user.email=probe@example.com", "commit", "-q", "-m", "probe")
		if added {
			// Added, it is a gitlink: listed without its trailing slash.
			git(t, dir, "add", "nested")
		}
		command := exec.Command("./scripts/repo-files.sh")
		command.Dir = dir
		if out, err := command.Output(); err == nil {
			t.Errorf("the list took a nested repository (added: %t): %q", added, out)
		}
	}
}

// Outside git a link is listed like a file, as git lists a tracked one, so a
// guard reads the name and what it points at.
func TestTheListOutsideGitNamesALink(t *testing.T) {
	parent := t.TempDir()
	git(t, parent, "init", "-q")
	if err := os.WriteFile(filepath.Join(parent, ".gitignore"), []byte("copy/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(parent, "copy")
	copyScripts(t, copied)
	if err := os.Symlink("repo-files.sh", filepath.Join(copied, "scripts", "linked.sh")); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("./scripts/repo-files.sh")
	command.Dir = copied
	out, err := command.Output()
	if err != nil || !strings.Contains(string(out), "scripts/linked.sh\n") {
		t.Errorf("the copy listed %q: %v", out, err)
	}
}
