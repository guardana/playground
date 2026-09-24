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
