package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scenarioTree(t *testing.T, files ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range files {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("schema_version: 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLocateFindsAScenarioByItsIdentifier(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml", "scenarios/auth/auth-01.yaml")

	found, err := locate(root, settings{scenario: "flow-02"})
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	want := filepath.Join(root, "scenarios/flow/flow-02.yaml")
	if len(found) != 1 || found[0] != want {
		t.Errorf("found %v, want [%s]", found, want)
	}
}

func TestLocateTakesAPathAsWritten(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml")
	path := filepath.Join(root, "scenarios/flow/flow-02.yaml")

	found, err := locate(root, settings{scenario: path})
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if len(found) != 1 || found[0] != path {
		t.Errorf("found %v, want [%s]", found, path)
	}
}

func TestLocateRefusesWhatItCannotResolve(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml", "scenarios/auth/flow-02.yaml")

	tests := []struct {
		name  string
		given settings
		says  string
	}{
		{"an identifier nothing on disk carries", settings{scenario: "flow-99"}, "flow-99"},
		{"an identifier two files claim", settings{scenario: "flow-02"}, "two"},
		{"a path that is not there", settings{scenario: filepath.Join(root, "scenarios/flow/nope.yaml")}, "nope"},
		{"neither a scenario nor -all", settings{}, "-scenario"},
		{"both a scenario and -all", settings{scenario: "flow-02", all: true}, "-all"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := locate(root, test.given)
			if err == nil {
				t.Fatalf("%+v resolved to something", test.given)
			}
			if !strings.Contains(err.Error(), test.says) {
				t.Errorf("the refusal is %v, want it to name %q", err, test.says)
			}
		})
	}
}

func TestLocateAllFindsEveryScenarioInOrder(t *testing.T) {
	root := scenarioTree(t,
		"scenarios/tool/tool-01.yaml",
		"scenarios/flow/flow-02.yaml",
		"scenarios/auth/auth-01.yaml",
		"scenarios/flow/README.md",
	)

	found, err := locate(root, settings{all: true})
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	want := []string{
		filepath.Join(root, "scenarios/auth/auth-01.yaml"),
		filepath.Join(root, "scenarios/flow/flow-02.yaml"),
		filepath.Join(root, "scenarios/tool/tool-01.yaml"),
	}
	if len(found) != len(want) {
		t.Fatalf("found %v, want %v", found, want)
	}
	for i := range want {
		if found[i] != want[i] {
			t.Errorf("position %d is %s, want %s", i, found[i], want[i])
		}
	}
}

// A run over nothing is not a green run.
func TestLocateAllRefusesAnEmptyCatalogue(t *testing.T) {
	if _, err := locate(scenarioTree(t), settings{all: true}); err == nil {
		t.Error("-all over a repository with no scenario found something to do")
	}
}

// A scenario outside the workspace would have the files it names read from
// the workspace, which is another scenario than the one written.
func TestLocateRefusesAPathOutsideTheWorkspace(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml")
	other := scenarioTree(t, "scenarios/flow/flow-03.yaml")
	link := filepath.Join(root, "scenarios/flow/linked.yaml")
	if err := os.Symlink(filepath.Join(other, "scenarios/flow/flow-03.yaml"), link); err != nil {
		t.Fatal(err)
	}
	for _, given := range []string{filepath.Join(other, "scenarios/flow/flow-03.yaml"), link, "../x/../" + filepath.Base(other) + "/scenarios/flow/flow-03.yaml"} {
		_, err := locate(root, settings{scenario: given})
		if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
			t.Errorf("-scenario %s was %v, want refused as outside the workspace", given, err)
		}
	}
}

func TestLocateReadsARelativePathInTheWorkspace(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml")
	found, err := locate(root, settings{scenario: "scenarios/flow/flow-02.yaml"})
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	if want := filepath.Join(root, "scenarios/flow/flow-02.yaml"); len(found) != 1 || found[0] != want {
		t.Errorf("found %v, want [%s]", found, want)
	}
}

// Found by identifier or by -all, a link out of the workspace is the same
// scenario it is when named by path, and is refused the same way.
func TestLocateRefusesALinkOutOfTheWorkspaceHoweverItIsFound(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml")
	other := scenarioTree(t, "scenarios/flow/flow-03.yaml")
	if err := os.Symlink(filepath.Join(other, "scenarios/flow/flow-03.yaml"), filepath.Join(root, "scenarios/flow/linked.yaml")); err != nil {
		t.Fatal(err)
	}
	for name, given := range map[string]settings{"by identifier": {scenario: "linked"}, "-all": {all: true}} {
		found, err := locate(root, given)
		if err == nil || !strings.Contains(err.Error(), "outside the workspace") {
			t.Errorf("%s found %v, %v; want the link refused as outside the workspace", name, found, err)
		}
	}
}
