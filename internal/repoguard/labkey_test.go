package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The key never lands inside the clone, however the path to it is spelled:
// through a link to the clone, through a link to a directory of the clone, or
// with the script itself reached through a link to the clone. Each is refused
// before docker is asked anything.
func TestALabKeyPathThroughALinkIntoTheCloneIsRefused(t *testing.T) {
	clone, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	links := t.TempDir()
	// A shell's $(...) drops trailing line breaks, so "L\n" is checked as the
	// directory L beside it while the key is written through the link.
	if err := os.Mkdir(filepath.Join(links, "L"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"clone": clone, "docs": filepath.Join(clone, "docs"), "L\n": clone} {
		if err := os.Symlink(target, filepath.Join(links, name)); err != nil {
			t.Fatal(err)
		}
	}
	for name, run := range map[string]struct{ script, keys, reason string }{
		"a link to the clone":       {clone, filepath.Join(links, "clone", "lab-key-probe"), "inside the clone"},
		"a link into the clone":     {clone, filepath.Join(links, "docs", "lab-key-probe"), "inside the clone"},
		"the script through a link": {filepath.Join(links, "clone"), filepath.Join(clone, "lab-key-probe"), "inside the clone"},
		"a line break in the path":  {clone, filepath.Join(links, "L\n", "lab-key-probe"), "control character"},
	} {
		t.Run(name, func(t *testing.T) {
			stubs := t.TempDir()
			marker := filepath.Join(stubs, "docker-ran")
			docker := "#!/bin/sh\ntouch '" + marker + "'\nexit 1\n"
			if err := os.WriteFile(filepath.Join(stubs, "docker"), []byte(docker), 0o700); err != nil { // #nosec G306 -- a stub to run.
				t.Fatal(err)
			}
			command := exec.Command("bash", filepath.Join(run.script, "scripts", "lab-key.sh")) // #nosec G204 -- paths of this test.
			command.Env = append(os.Environ(), "LAB_KEYS_DIR="+run.keys,
				"PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(out), run.reason) {
				t.Errorf("LAB_KEYS_DIR=%s: %v\n%s", run.keys, err, out)
			}
			if _, stat := os.Stat(marker); stat == nil {
				t.Error("docker was asked before the place was refused")
			}
		})
	}
	for _, probe := range []string{filepath.Join(clone, "lab-key-probe"), filepath.Join(clone, "docs", "lab-key-probe")} {
		if _, err := os.Lstat(probe); err == nil {
			t.Errorf("%s exists after the run", probe)
		}
	}
}

// A place outside the clone passes the check and reaches docker, here a stub
// that answers no image, so the refusals above are not a script refusing all.
func TestALabKeyPlaceOutsideTheCloneReachesDocker(t *testing.T) {
	stubs := t.TempDir()
	marker := filepath.Join(stubs, "docker-ran")
	docker := "#!/bin/sh\ntouch '" + marker + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "docker"), []byte(docker), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	keys := filepath.Join(t.TempDir(), "state", "lab-key")
	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "lab-key.sh")) // #nosec G204 -- a path of this test.
	command.Env = append(os.Environ(), "LAB_KEYS_DIR="+keys, "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	if _, stat := os.Stat(marker); stat != nil || err == nil || !strings.Contains(string(out), "no image") {
		t.Errorf("LAB_KEYS_DIR=%s: %v\n%s", keys, err, out)
	}
	if _, stat := os.Stat(filepath.Dir(keys)); stat == nil {
		t.Error("the key's parent was made although no key was")
	}
}

// A . component makes mv put the key one directory below the place named, so
// the next run finds no key there and refuses.
func TestALabKeyPathWithADotComponentIsRefused(t *testing.T) {
	stubs := t.TempDir()
	marker := filepath.Join(stubs, "docker-ran")
	docker := "#!/bin/sh\ntouch '" + marker + "'\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "docker"), []byte(docker), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	keys := filepath.Join(t.TempDir(), "state") + "/."
	command := exec.Command("bash", filepath.Join(repoRoot, "scripts", "lab-key.sh")) // #nosec G204 -- a path of this test.
	command.Env = append(os.Environ(), "LAB_KEYS_DIR="+keys, "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "component") {
		t.Errorf("LAB_KEYS_DIR=%s: %v\n%s", keys, err, out)
	}
	if _, stat := os.Stat(marker); stat == nil {
		t.Error("docker was asked before the place was refused")
	}
}
