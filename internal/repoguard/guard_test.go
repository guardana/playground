package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const repoRoot = "../.."

// Each guard is checked against a planted violation. A guard that has only ever
// been seen to pass is not known to work, and this repository's own rule is
// that a check which cannot fail does not count as a check.
func TestGuardsRejectWhatTheyClaimTo(t *testing.T) {
	// The guards scan every file, this one included, so each sample is
	// assembled from pieces that do not match on their own.
	cases := []struct {
		name    string
		script  string
		file    string
		content string
	}{
		{"tool attribution", "check-attribution.sh", "probe.md", "Co-Authored" + "-By: A Tool <tool@example.com>\n"},
		{"assistant vendor name", "check-attribution.sh", "probe.md", "Written during an Anthro" + "pic experiment.\n"},
		{"generated-with credit", "check-attribution.sh", "probe.md", "Generated " + "with a coding assistant.\n"},
		{"text outside English", "check-hygiene.sh", "probe.md", "Za\u017c\u00f3\u0142\u0107 g\u0119\u015bl\u0105 ja\u017a\u0144.\n"},
		{"leftover placeholder", "check-hygiene.sh", "probe.md", "Release date: TB" + "D\n"},
		{"working material", "check-hygiene.sh", "SPEC-draft-2026.md", "notes\n"},
		{"output from a run", "check-hygiene.sh", "docs/re" + "ports/run-2026.md", "scenario: pass\n"},
		{"local tooling", "check-hygiene.sh", "." + "envrc", "export LAB_DEBUG=1\n"},
		{"local tooling below the root", "check-hygiene.sh", "victims/fs/." + "toolrc", "export LAB_DEBUG=1\n"},
		{"a root name that only begins like git's own", "check-hygiene.sh", "." + "gitlocal", "notes\n"},
		{"local tooling under the workflows' directory", "check-hygiene.sh", ".github/." + "toolrc", "export LAB_DEBUG=1\n"},
		{"local tooling under a name git would quote", "check-hygiene.sh", "victims/fs/." + "caf\u00e9rc", "export LAB_DEBUG=1\n"},
		{"a macOS home directory", "check-hygiene.sh", "probe.md", "Keys live in /Us" + "ers/someone/keys.\n"},
		{"a Linux home directory", "check-hygiene.sh", "probe.md", "Clone into /ho" + "me/someone/src.\n"},
		{"a Linux home directory at the end of a line", "check-hygiene.sh", "probe.md", "HOME=/ho" + "me/someone\n"},
		{"a home directory after the distroless one, by a space", "check-hygiene.sh", "probe.md", "HOME=/ho" + "me/nonroot /ho" + "me/someone/x\n"},
		{"a home directory after the distroless one, by a colon", "check-hygiene.sh", "probe.md", "PATH=/ho" + "me/nonroot:/ho" + "me/someone/bin\n"},
		{"the macOS /tmp by its real path", "check-hygiene.sh", "probe.md", "Reports in /priv" + "ate/tmp/run-1.\n"},
		{"a macOS per-user temporary directory", "check-hygiene.sh", "probe.md", "Left in /va" + "r/folders/7x/T/run.\n"},
		{"a macOS per-user temporary directory by its real path", "check-hygiene.sh", "probe.md", "At /priv" + "ate/var/folders/7x/T.\n"},
		{"the enforcer's sibling checkout", "check-hygiene.sh", "probe.md", "Build from ../con" + "trol at the pin.\n"},
		{"the verifier's sibling checkout", "check-hygiene.sh", "probe.md", "Read ../guar" + "dana/docs first.\n"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			planted := filepath.Join(repoRoot, test.file)
			// A case may name a directory this repository no longer has. Create
			// it only if it is missing, and then take it away again.
			if directory := filepath.Dir(planted); directory != repoRoot {
				if _, err := os.Stat(directory); os.IsNotExist(err) {
					if err := os.MkdirAll(directory, 0o750); err != nil {
						t.Fatal(err)
					}
					defer func() {
						if err := os.Remove(directory); err != nil {
							t.Errorf("planted directory left behind: %v", err)
						}
					}()
				}
			}
			if _, err := os.Stat(planted); err == nil {
				t.Fatalf("%s already exists; refusing to overwrite", test.file)
			}
			if err := os.WriteFile(planted, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Remove(planted); err != nil {
					t.Errorf("planted file left behind: %v", err)
				}
			}()

			if err := runGuard(test.script); err == nil {
				t.Errorf("%s accepted %q", test.script, test.content)
			}
		})
	}
}

// A guard that refuses text a stranger's repository legitimately holds is
// switched off by the first person it blocks, so each near miss must pass.
// Each is split like the refused samples, so it is judged only when planted.
func TestHygieneTakesPathsThatAreNoMaintainersMachine(t *testing.T) {
	for _, content := range []string{
		"The distroless image runs with HOME=/ho" + "me/nonroot/ and /ho" + "me/nonroot\n",
		"See ../con" + "trol-plane.md and ../guar" + "dana-gateway/ for the layout.\n",
		"The API answers at https://example.com/Us" + "ers/ and https://example.com/ho" + "me/x/.\n",
	} {
		planted := filepath.Join(repoRoot, "probe.md")
		if _, err := os.Stat(planted); err == nil {
			t.Fatal("probe.md already exists; refusing to overwrite")
		}
		if err := os.WriteFile(planted, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		err := runGuard("check-hygiene.sh")
		if removed := os.Remove(planted); removed != nil {
			t.Errorf("planted file left behind: %v", removed)
		}
		if err != nil {
			t.Errorf("check-hygiene.sh refused %q: %v", content, err)
		}
	}
}

// Discovered rather than listed, so a guard added tomorrow is covered today.
func TestGuardsPassOnTheRepositoryAsItStands(t *testing.T) {
	scripts, err := filepath.Glob(filepath.Join(repoRoot, "scripts", "check-*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("no guards found; this test inspected nothing")
	}
	for _, script := range scripts {
		name := filepath.Base(script)
		if err := runGuard(name); err != nil {
			t.Errorf("%s failed on a clean tree: %v", name, err)
		}
	}
}

func runGuard(script string) error {
	// #nosec G204 -- script names come from the literal lists above.
	command := exec.Command("./scripts/" + script)
	command.Dir = repoRoot
	return command.Run()
}
