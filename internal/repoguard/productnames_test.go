package repoguard_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The product-name guard scans this file too, so every name it refuses is
// assembled here from pieces that do not match on their own.
const (
	product  = "guar" + "dana"
	roleWord = "Con" + "trol"
	module   = "github.com/" + product + "/playground"
	allow    = "scripts/product-names-allow.txt"
	// plantedAllow allows the planted tree's driver, its pins and the guard.
	plantedAllow = "drivers/**  the driver\nversions.env  the pins\nscripts/check-product-names.sh  the rule\n"
)

// productTree is a repository of its own for the guard: its pins name a
// fictional tool, so a refusal of that tool's names can only come from
// versions.env, and its one driver and its pins are the allowed places.
func productTree() map[string]string {
	return map[string]string{
		"versions.env": "VERIFIER_PACKAGE=acme-cli\nVERIFIER_CLI=acme\nENFORCER_GATEWAY_BIN=acme-gateway\n" +
			"ENFORCER_CONTROL_BIN=acme-gate\nENFORCER_NAMESPACE=acme.gate\nENFORCER_REPOSITORY=https://example.com/acme/gate\n",
		allow:                               "# planted\n" + plantedAllow,
		"drivers/" + product + "/driver.go": "package " + product + "\n\n// Driver runs acme-gateway.\ntype Driver struct{}\n",
		"runner/x.go":                       "package main\n\n// the enforcer decides\n",
	}
}

// Each case writes one file into the planted tree; an empty reason is a pass.
func TestTheProductNameGuardRefusesWhatItClaimsTo(t *testing.T) {
	for _, test := range []struct{ name, path, body, reason string }{
		{"the planted tree as it is", "", "", ""},
		{"a product name in neutral code", "runner/x.go", "package main\n\n// " + product + " decides\n", "runner/x.go:3:"},
		{"the same line in a driver", "drivers/x/x.go", "package x\n\n// " + product + " decides\n", ""},
		{"the role word in neutral code", "runner/x.go", "package main\n\n// " + roleWord + " decides\n", "runner/x.go:3:"},
		{"the repository's own identity", "runner/check/x.go", "package check\n\nimport _ \"" + module + "/internal/x\"\n\n// io." +
			product + ".playground/step, " + product + "-playground, @" + product + "/maintainers\n", ""},
		{"a driver wired in the runner's main package", "runner/x.go", "package main\n\nimport \"" + module + "/drivers/" +
			product + "\"\n\nvar d *" + product + ".Driver\n", ""},
		{"a driver named in a comment where drivers are wired", "runner/x.go", "package main\n\n// " + product +
			".Verdict decides here\n", "runner/x.go:3:"},
		{"a driver named in a trailing comment where drivers are wired", "runner/x.go", "package main\n\nvar d *" + product +
			".Driver // " + product + ".Verdict decides here\n", "runner/x.go:3:"},
		{"a driver named outside a main package", "runner/check/x.go", "package check\n\nimport \"" + module + "/drivers/" +
			product + "\"\n", "runner/check/x.go:3:"},
		{"a name derived from versions.env", "runner/x.go", "package main\n\n// runs acme-gateway\n", "runner/x.go:3:"},
		{"an entry matching no file", allow, plantedAllow + "docs/**  pages\n", "matches no file: docs/**"},
		{"an entry covering no named line", allow, plantedAllow + "runner/**  r\n", "covers no line naming a system under test: runner/**"},
		{"an entry without a reason", allow, plantedAllow + "README.md\n", "without a reason: README.md"},
		{"a pin missing from versions.env", "versions.env", "VERIFIER_PACKAGE=acme-cli\n", "versions.env sets no VERIFIER_CLI"},
		{"no name derived from versions.env", "versions.env", "VERIFIER_PACKAGE=\nVERIFIER_CLI=\nENFORCER_GATEWAY_BIN=\n" +
			"ENFORCER_CONTROL_BIN=\nENFORCER_NAMESPACE=\nENFORCER_REPOSITORY=\n", "versions.env gives VERIFIER_PACKAGE no name"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			tree := productTree()
			if test.path != "" {
				tree[test.path] = test.body
			}
			out, err := runProductNames(t, tree)
			switch {
			case test.reason == "" && err != nil:
				t.Errorf("refused: %v\n%s", err, out)
			case test.reason != "" && err == nil:
				t.Errorf("accepted, want a refusal naming %q\n%s", test.reason, out)
			case test.reason != "" && !strings.Contains(out, test.reason):
				t.Errorf("refused for another reason, want %q\n%s", test.reason, out)
			}
		})
	}
}

// runProductNames runs the guard on tree, written into a directory outside any
// work tree, where scripts/repo-files.sh lists files with find.
func runProductNames(t *testing.T, tree map[string]string) (string, error) {
	t.Helper()
	root := t.TempDir()
	for _, script := range []string{"check-product-names.sh", "repo-files.sh"} {
		body, err := os.ReadFile(filepath.Join(repoRoot, "scripts", script))
		if err != nil {
			t.Fatal(err)
		}
		tree["scripts/"+script] = string(body)
	}
	for name, body := range tree {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil { // #nosec G306 -- the scripts must run.
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "bash", filepath.Join(root, "scripts", "check-product-names.sh")) // #nosec G204 -- a path of this test.
	out, err := command.CombinedOutput()
	return string(out), err
}
