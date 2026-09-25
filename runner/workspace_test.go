package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

func TestNoWorkspaceMeansTheClone(t *testing.T) {
	root := t.TempDir()
	got, err := openWorkspace(root, filepath.Join(root, "reports"), []string{"HOME=/nowhere"})
	if err != nil {
		t.Fatalf("openWorkspace: %v", err)
	}
	if got.external || got.dir != resolved(root) {
		t.Errorf("an unset LAB_WORKSPACE opened %+v, want the clone %s", got, resolved(root))
	}
}

func TestAWorkspaceOutsideTheCloneIsTakenByItsResolvedPath(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "ws")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	got, err := openWorkspace(root, filepath.Join(root, "reports"), []string{"LAB_WORKSPACE=" + link})
	if err != nil {
		t.Fatalf("openWorkspace: %v", err)
	}
	want, _ := filepath.EvalSymlinks(outside)
	if !got.external || got.dir != want {
		t.Errorf("LAB_WORKSPACE=%s opened %+v, want %s", link, got, want)
	}
}

func TestAWorkspaceIsRefusedBeforeAnythingRuns(t *testing.T) {
	root, elsewhere := t.TempDir(), t.TempDir()
	reports := filepath.Join(elsewhere, "reports")
	writeFile(filepath.Join(root, "scenarios/tool/x.yaml"), "schema_version: 1\n")
	writeFile(filepath.Join(reports, "mine/scenarios/tool/x.yaml"), "schema_version: 1\n")
	writeFile(filepath.Join(elsewhere, "a-file"), "not a directory\n")
	intoClone := filepath.Join(elsewhere, "into-clone")
	if err := os.Symlink(filepath.Join(root, "scenarios"), intoClone); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, value, says string }{
		{"set and empty", "", "empty"},
		{"set to blanks", "  ", "empty"},
		{"a path that is not there", filepath.Join(elsewhere, "absent"), "absent"},
		{"a file", filepath.Join(elsewhere, "a-file"), "not a directory"},
		{"the clone itself", root, "inside the clone"},
		{"a directory in the clone", filepath.Join(root, "scenarios"), "inside the clone"},
		{"a link into the clone", intoClone, "inside the clone"},
		{"a directory in the reports", filepath.Join(reports, "mine"), "inside the reports directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := openWorkspace(root, reports, []string{"LAB_WORKSPACE=" + test.value})
			if err == nil {
				t.Fatalf("LAB_WORKSPACE=%q was taken", test.value)
			}
			if !strings.Contains(err.Error(), test.says) {
				t.Errorf("the refusal is %v, want it to say %q", err, test.says)
			}
		})
	}
}

func everyNamedFile() labspec.Scenario {
	return labspec.Scenario{
		Trajectory: "trajectories/t.yaml",
		Gateway: &labspec.Gateway{
			Config: "config/gateway/scenarios/g.yaml", Policy: "config/policies/p.json",
			PDPScript: "d.yaml", ApproverScript: "a.yaml",
		},
		Trace: &labspec.Trace{Contract: "config/contracts/c.yaml", AISystem: "lab"},
	}
}

var namedPaths = map[string]string{
	"trajectory":              "trajectories/t.yaml",
	"gateway.config":          "config/gateway/scenarios/g.yaml",
	"gateway.policy":          "config/policies/p.json",
	"gateway.pdp_script":      "config/pdp/d.yaml",
	"gateway.approver_script": "config/approver/a.yaml",
	"trace.contract":          "config/contracts/c.yaml",
}

func workspaceHolding(t *testing.T, except string) workspace {
	t.Helper()
	dir := t.TempDir()
	for field, path := range namedPaths {
		if field != except {
			writeFile(filepath.Join(dir, path), "x\n")
		}
	}
	return workspace{dir: resolved(dir), external: true}
}

func TestAWorkspaceHoldingEveryNamedFileIsTaken(t *testing.T) {
	if err := workspaceHolding(t, "").refuseMissing(everyNamedFile()); err != nil {
		t.Errorf("a workspace holding every file was refused: %v", err)
	}
}

func TestAScenarioNamingAFileTheWorkspaceLacksIsRefused(t *testing.T) {
	for field, path := range namedPaths {
		t.Run(field, func(t *testing.T) {
			err := workspaceHolding(t, field).refuseMissing(everyNamedFile())
			if err == nil {
				t.Fatalf("a workspace without %s was taken", path)
			}
			if !strings.Contains(err.Error(), field) || !strings.Contains(err.Error(), path) {
				t.Errorf("the refusal is %v, want it to name %s and %s", err, field, path)
			}
		})
	}
}

// A container sees one directory of the workspace; a link out of it would be
// read by the runner and be missing, or another file, in the container.
func TestANamedFileThatLinksOutOfItsDirectoryIsRefused(t *testing.T) {
	space := workspaceHolding(t, "trajectory")
	target := filepath.Join(space.dir, "config/policies/p.json")
	if err := os.MkdirAll(filepath.Join(space.dir, "trajectories"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(space.dir, "trajectories/t.yaml")); err != nil {
		t.Fatal(err)
	}
	err := space.refuseMissing(everyNamedFile())
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("a trajectory linking to a policy was %v, want refused as outside trajectories", err)
	}
}

func TestANamedDirectoryIsNotAFile(t *testing.T) {
	space := workspaceHolding(t, "trajectory")
	if err := os.MkdirAll(filepath.Join(space.dir, "trajectories/t.yaml"), 0o750); err != nil {
		t.Fatal(err)
	}
	err := space.refuseMissing(everyNamedFile())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("a trajectory that is a directory was %v", err)
	}
}

// Compose binds a workspace directory by the path as written and Docker follows
// a link there, so a linked directory would show a container what it points at.
func TestAWorkspaceDirectoryThatIsALinkIsRefused(t *testing.T) {
	for _, dir := range []string{
		"trajectories", "config", "config/contracts", "config/pdp", "config/approver",
		"config/policies", "config/gateway/scenarios",
	} {
		t.Run(dir, func(t *testing.T) {
			space := workspaceHolding(t, "")
			moved := filepath.Join(t.TempDir(), "moved")
			if err := os.Rename(filepath.Join(space.dir, dir), moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(moved, filepath.Join(space.dir, dir)); err != nil {
				t.Fatal(err)
			}
			err := space.refuseMissing(labspec.Scenario{Trajectory: "trajectories/t.yaml"})
			if err == nil || !strings.Contains(err.Error(), "is a link") {
				t.Errorf("a workspace whose %s links to %s was %v, want refused as a link", dir, moved, err)
			}
		})
	}
}

func TestADanglingWorkspaceLinkIsRefused(t *testing.T) {
	space := workspaceHolding(t, "")
	link := filepath.Join(space.dir, "config/pdp")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "absent"), link); err != nil {
		t.Fatal(err)
	}
	err := space.refuseMissing(labspec.Scenario{Trajectory: "trajectories/t.yaml"})
	if err == nil || !strings.Contains(err.Error(), "is a link") {
		t.Errorf("a dangling config/pdp link was %v, want refused as a link", err)
	}
}

// Services write every run's pki, gateway directory and journals into the
// reports, and a workspace directory is mounted into the agent.
func TestReportsInsideADirectoryTheCloneMountsAreRefused(t *testing.T) {
	root := t.TempDir()
	for _, inside := range []string{"trajectories/out", "config/pdp/out"} {
		reports := filepath.Join(root, inside)
		_, err := openWorkspace(root, reports, nil)
		if err == nil || !strings.Contains(err.Error(), "which containers mount") {
			t.Errorf("-reports %s in the clone was %v, want refused", inside, err)
		}
	}
	if _, err := openWorkspace(root, filepath.Join(root, "reports"), nil); err != nil {
		t.Errorf("the default reports directory was refused: %v", err)
	}
}

func TestReportsInsideTheWorkspaceAreRefused(t *testing.T) {
	root, space := t.TempDir(), t.TempDir()
	for _, reports := range []string{filepath.Join(space, "trajectories", "out"), space} {
		_, err := openWorkspace(root, reports, []string{"LAB_WORKSPACE=" + space})
		if err == nil || !strings.Contains(err.Error(), "reports directory "+reports+" is inside the workspace") {
			t.Errorf("-reports %s with the workspace %s was %v, want refused", reports, space, err)
		}
	}
}
