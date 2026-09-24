package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// apart moves the lab's own files into a clone of their own and leaves the
// rest where it was, as the workspace, so neither directory holds a file the
// other should have been read from.
func apart(t *testing.T, subject *lab, labOwned ...string) {
	t.Helper()
	clone := t.TempDir()
	for _, file := range labOwned {
		to := filepath.Join(clone, file)
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			t.Fatal(err)
		}
		err := os.Rename(filepath.Join(subject.root, file), to)
		// Not every enforcer lab writes a versions.env; one that does keeps it in the clone.
		if err != nil && (file != versionFile || !errors.Is(err, fs.ErrNotExist)) {
			t.Fatal(err)
		}
	}
	subject.workspace = workspace{dir: resolved(subject.root)}
	subject.root = clone
}

func TestAScenariosFilesAreReadFromTheWorkspaceAndTheLabsFromTheClone(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	apart(t, &subject, classification, fingerprints, versionFile)
	var signed string
	sign := subject.sign
	subject.sign = func(ctx context.Context, keys, policy, out string) error {
		signed = policy
		return sign(ctx, keys, policy, out)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() != assertion.Pass {
		for _, result := range graded.Results {
			t.Logf("%s %s: %s %s", result.Check, result.Outcome, result.Got, result.Detail)
		}
		t.Fatalf("a run split between a clone and a workspace is %s", graded.Outcome())
	}
	if want := filepath.Join(subject.workspace.dir, "config/policies/flow-01.json"); signed != want {
		t.Errorf("signed %s, want the workspace's %s", signed, want)
	}
	if got := compose.env[workspaceVariable]; got != subject.workspace.dir {
		t.Errorf("compose mounts the workspace from %q, want %q", got, subject.workspace.dir)
	}
}

func TestAScenarioNamingAFileTheWorkspaceLacksBringsNothingUp(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	if err := os.Remove(filepath.Join(subject.root, "config/policies/flow-01.json")); err != nil {
		t.Fatal(err)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() != assertion.Fail {
		t.Fatalf("a run without its policy is %s", graded.Outcome())
	}
	found := false
	for _, result := range graded.Results {
		found = found || strings.Contains(result.Detail, "gateway.policy config/policies/flow-01.json is not in the workspace")
	}
	if !found {
		t.Errorf("no result names the missing policy: %+v", graded.Results)
	}
	if len(compose.calls) != 0 {
		t.Errorf("a refused run called docker: %v", compose.calls)
	}
}

func TestRunLocatesScenariosOnlyInTheWorkspace(t *testing.T) {
	root, space := t.TempDir(), t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=c0ffee\n")
	writeFile(filepath.Join(root, "scenarios/flow/flow-01.yaml"), scenarioFile)
	t.Chdir(root)
	for _, test := range []struct {
		name string
		args []string
		says string
	}{
		{"by identifier", []string{"-scenario", "flow-01"}, filepath.Join(resolved(space), "scenarios")},
		{"by the clone's path", []string{"-scenario", filepath.Join(root, "scenarios/flow/flow-01.yaml")}, "outside the workspace"},
		{"all of them", []string{"-all"}, filepath.Join(resolved(space), "scenarios")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			err := run(context.Background(), test.args, []string{workspaceVariable + "=" + space}, &out)
			if err == nil || !strings.Contains(err.Error(), test.says) {
				t.Errorf("run = %v, want a refusal saying %q", err, test.says)
			}
		})
	}
}

func TestRunRefusesAWorkspaceInsideTheReportsBeforeLookingForAScenario(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=c0ffee\n")
	space := filepath.Join(elsewhere, "reports", "ws")
	writeFile(filepath.Join(space, "scenarios/flow/flow-01.yaml"), scenarioFile)
	t.Chdir(root)
	var out strings.Builder
	err := run(context.Background(), []string{"-scenario", "flow-01", "-reports", filepath.Join(elsewhere, "reports")},
		[]string{workspaceVariable + "=" + space}, &out)
	if err == nil || !strings.Contains(err.Error(), "inside the reports directory") {
		t.Errorf("run = %v, want a refusal naming the reports directory", err)
	}
}
