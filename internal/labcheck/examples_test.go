package labcheck_test

import (
	"crypto/sha256"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// examplesDir holds worked examples, each laid out as a workspace that
// LAB_WORKSPACE names once it is copied out of the clone.
const examplesDir = "examples"

// exampleReadme is the one file of an example no scenario names.
const exampleReadme = "README.md"

// An example is run from a copy outside the clone, where nothing in the lab
// looks at it before a user boots it: every scenario loads and agrees with its
// trajectory, is decided by the enforcer, names only files the example holds,
// and every file the example holds is named.
func TestEveryExampleIsAWorkspaceThatLoads(t *testing.T) {
	examples := exampleDirs(t)
	if len(examples) == 0 {
		t.Fatal("no example under examples/; this guard inspected nothing")
	}
	for _, dir := range examples {
		t.Run(relative(dir), func(t *testing.T) {
			files := workspaceFiles(t, dir)
			used := map[string]bool{exampleReadme: true}
			scenarios := 0
			for _, name := range files {
				if !isWorkspaceScenario(name) {
					continue
				}
				scenarios++
				used[name] = true
				for _, named := range checkWorkspaceScenario(t, dir, name) {
					used[named] = true
				}
			}
			if scenarios == 0 {
				t.Fatalf("%s holds no scenarios/<class>/<id>.yaml", relative(dir))
			}
			if !slices.Contains(files, exampleReadme) {
				t.Errorf("%s has no %s", relative(dir), exampleReadme)
			}
			for _, name := range files {
				if !used[name] {
					t.Errorf("%s/%s is no scenario's", relative(dir), name)
				}
			}
		})
	}
}

// An example is the adopter's own files, not the lab's under another path: a
// copy would show nothing the lab's catalogue does not already run.
func TestNoExampleFileIsACopyOfALabFile(t *testing.T) {
	lab := map[[sha256.Size]byte]string{}
	for _, directory := range []string{"scenarios", "trajectories", "config"} {
		root := filepath.Join(repoRoot, directory)
		for _, name := range workspaceFiles(t, root) {
			lab[digest(t, filepath.Join(root, name))] = path.Join(directory, name)
		}
	}
	examples := exampleDirs(t)
	if len(examples) == 0 {
		t.Fatal("no example under examples/; this guard inspected nothing")
	}
	for _, dir := range examples {
		for _, name := range workspaceFiles(t, dir) {
			if original, copied := lab[digest(t, filepath.Join(dir, name))]; copied {
				t.Errorf("%s/%s is a copy of %s", relative(dir), name, original)
			}
		}
	}
}

// checkWorkspaceScenario loads one scenario of an example the way the runner
// reads it from a workspace and returns the files it names, workspace-relative.
func checkWorkspaceScenario(t *testing.T, dir, name string) []string {
	t.Helper()
	scenario, err := labspec.LoadScenario(filepath.Join(dir, name))
	if err != nil {
		t.Errorf("LoadScenario: %v", err)
		return nil
	}
	if !scenario.UsesEnforcer() {
		t.Errorf("%s is not decided by the enforcer (gateway:); a workspace scenario always is", name)
		return nil
	}
	trajectory, err := labspec.LoadTrajectory(filepath.Join(dir, filepath.FromSlash(scenario.Trajectory)))
	if err != nil {
		t.Errorf("LoadTrajectory %s: %v", scenario.Trajectory, err)
	} else if err := labspec.Validate(scenario, trajectory); err != nil {
		t.Errorf("Validate %s: %v", name, err)
	}
	var named []string
	for file, under := range namedByScenario(scenario) {
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil || !info.Mode().IsRegular() {
			t.Errorf("%s names %s, which the example does not hold as a regular file", name, file)
		}
		// The runner reads each file only from the directory it belongs in.
		if !strings.HasPrefix(file, under) || strings.Contains(file, "..") {
			t.Errorf("%s names %s outside %s, where the runner reads it from", name, file, under)
		}
		named = append(named, file)
	}
	return named
}

// namedByScenario maps every file a scenario the enforcer decides names to the
// workspace directory it has to sit in.
func namedByScenario(scenario labspec.Scenario) map[string]string {
	named := map[string]string{
		scenario.Trajectory:     "trajectories/",
		scenario.Gateway.Config: labspec.GatewayConfigDir,
		scenario.Gateway.Policy: labspec.PolicyDir,
	}
	if script := scenario.Gateway.PDPScript; script != "" {
		named[pdpScriptDir+script] = pdpScriptDir
	}
	if script := scenario.Gateway.ApproverScript; script != "" {
		named[approverScriptDir+script] = approverScriptDir
	}
	if scenario.Trace != nil {
		named[scenario.Trace.Contract] = labspec.ContractDir
	}
	return named
}

// isWorkspaceScenario matches what the runner looks for in a workspace:
// scenarios/<class>/<id>.yaml, one level deep.
func isWorkspaceScenario(name string) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 3 && parts[0] == "scenarios" && strings.HasSuffix(parts[2], ".yaml")
}

func exampleDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, examplesDir))
	if err != nil {
		t.Fatalf("%s: %v", examplesDir, err)
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("%s/%s is not an example directory", examplesDir, entry.Name())
			continue
		}
		dirs = append(dirs, filepath.Join(repoRoot, examplesDir, entry.Name()))
	}
	return dirs
}

// workspaceFiles lists every file under root, slash-separated and relative to
// it. A link is refused: the runner refuses one on the way to a mounted
// directory, so an example holding one would not run once copied.
func workspaceFiles(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(at string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case entry.Type()&fs.ModeSymlink != 0:
			t.Errorf("%s is a link", relative(at))
		case !entry.IsDir():
			name, err := filepath.Rel(root, at)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(name))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func digest(t *testing.T, file string) [sha256.Size]byte {
	t.Helper()
	body, err := os.ReadFile(file) // #nosec G304 -- a file of this repository.
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(body)
}
