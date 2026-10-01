package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A download whose sha256 is not the pinned one is refused from inside fetch,
// and the refusal exits the script; what fetch downloaded goes with it.
func TestARefusedDownloadLeavesNothingBehind(t *testing.T) {
	stubs, scratch := t.TempDir(), t.TempDir()
	marker := filepath.Join(stubs, "curl-ran")
	curl := "#!/bin/sh\ntouch '" + marker + "'\n" +
		"while [ $# -gt 0 ]; do\n\tif [ \"$1\" = -o ]; then printf 'not the release' >\"$2\"; exit 0; fi\n\tshift\ndone\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubs, "curl"), []byte(curl), 0o700); err != nil { // #nosec G306 -- a stub to run.
		t.Fatal(err)
	}
	// #nosec G204 -- fixed arguments.
	command := exec.Command("bash", "-c",
		`source scripts/bootstrap.sh && source scripts/tool-versions.env && fetch gitleaks amd64 "$1"`,
		"bootstrap", filepath.Join(t.TempDir(), "bin"))
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+scratch)
	out, err := command.CombinedOutput()
	if _, stat := os.Stat(marker); stat != nil {
		t.Fatalf("the stub curl did not run: %s", out)
	}
	if err == nil || !strings.Contains(string(out), "sha256 mismatch") || strings.Contains(string(out), "unbound") {
		t.Fatalf("fetch of a download no pin matches: %v\n%s", err, out)
	}
	left, err := os.ReadDir(scratch)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range left {
		t.Errorf("left behind in TMPDIR: %s", entry.Name())
	}
}
