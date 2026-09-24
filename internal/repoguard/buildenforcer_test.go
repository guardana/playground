package repoguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The enforcer image carries the tree the build script verified, and the
// runner trusts that label; a script that labelled an unverified tree would
// turn every plane/image check into a formality.
func TestTheEnforcerBuildLabelsOnlyThePinnedTree(t *testing.T) {
	source, commit, tree := enforcerSource(t)
	for name, test := range map[string]struct {
		pinned string
		built  bool
	}{
		"the pinned tree":   {tree, true},
		"another tree":      {strings.Repeat("5", 40), false},
		"no tree is pinned": {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			root, calls := buildTree(t, commit, test.pinned)
			command := exec.Command(filepath.Join(root, "scripts", "build-enforcer.sh")) // #nosec G204 -- the script copied into the test's own directory.
			command.Env = append(os.Environ(), "ENFORCER_SOURCE="+source,
				"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := command.CombinedOutput()
			called, _ := os.ReadFile(calls) // #nosec G304 -- written by the test's own docker stand-in.
			label := "--label io.guardana.playground.enforcer.tree=" + tree
			switch {
			case test.built && (err != nil || !strings.Contains(string(called), label)):
				t.Errorf("the pinned tree did not build with %q (%v):\n%s\n%s", label, err, output, called)
			case !test.built && (err == nil || len(called) > 0):
				t.Errorf("a build pinned to %q was not refused before docker (%v):\n%s\n%s", test.pinned, err, output, called)
			}
		})
	}
}

// enforcerSource is a repository of one commit, standing in for the enforcer.
func enforcerSource(t *testing.T) (dir, commit, tree string) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
		// #nosec G204 -- git on the test's own repository, with arguments the test writes.
		command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=lab", "-c", "user.email=lab@example.invalid"}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module enforcer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "go.mod")
	git("commit", "-q", "-m", "one")
	return dir, git("rev-parse", "HEAD"), git("rev-parse", "HEAD^{tree}")
}

// buildTree is the script beside a versions.env pinning commit and tree, and a
// docker stand-in that records its arguments.
func buildTree(t *testing.T, commit, tree string) (root, calls string) {
	t.Helper()
	root = t.TempDir()
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "build-enforcer.sh"))
	if err != nil {
		t.Fatal(err)
	}
	calls = filepath.Join(root, "docker-calls")
	pins := "ENFORCER_COMMIT=" + commit + "\nENFORCER_IMAGE=lab-enforcer\nENFORCER_GATEWAY_BIN=gateway\n" +
		"ENFORCER_CONTROL_BIN=control\nGO_BUILD_IMAGE=go\nSERVICE_BASE_IMAGE=base\n"
	if tree != "" {
		pins += "ENFORCER_TREE=" + tree + "\n"
	}
	for path, body := range map[string]string{
		"scripts/build-enforcer.sh": string(script),
		"versions.env":              pins,
		"bin/docker":                "#!/bin/sh\necho \"$*\" >> '" + calls + "'\necho sha256:0\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o700); err != nil { // #nosec G306 -- both scripts are executed.
			t.Fatal(err)
		}
	}
	return root, calls
}
