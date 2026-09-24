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
