package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A file under a suite that the -all glob does not match would be a scenario
// someone wrote and nobody ran; -all names it instead of passing without it.
func TestLocateAllRefusesAFileItWouldNotRun(t *testing.T) {
	for name, stray := range map[string]string{
		"a .yml":                         "scenarios/flow/flow-03.yml",
		"a nested .yaml":                 "scenarios/flow/more/flow-04.yaml",
		"a README":                       "scenarios/flow/README.md",
		"no extension":                   "scenarios/auth/auth-02",
		"a .yaml.disabled":               "scenarios/auth/auth-03.yaml.disabled",
		"a scenario outside every suite": "scenarios/flow-05.yaml",
		"an upper-case .YAML outside every suite": "scenarios/flow-06.YAML",
		"a .Yml outside every suite":              "scenarios/flow-07.Yml",
		"a hidden .yml":                           "scenarios/flow/.flow-08.yml",
		"a hidden .YAML":                          "scenarios/flow/.flow-09.YAML",
		"a hidden directory in a suite":           "scenarios/flow/.drafts/flow-10.yaml",
	} {
		t.Run(name, func(t *testing.T) {
			root := scenarioTree(t, "scenarios/flow/flow-02.yaml", "scenarios/auth/auth-01.yaml", stray)
			found, err := locate(root, settings{all: true})
			if err == nil {
				t.Fatalf("-all ran %v beside %s", found, stray)
			}
			named := filepath.Join(root, stray)
			if name == "a nested .yaml" || name == "a hidden directory in a suite" {
				named = filepath.Dir(named)
			}
			if !strings.Contains(err.Error(), named) || !strings.Contains(err.Error(), "-all would not run") {
				t.Errorf("the refusal is %v, want it to name %s", err, named)
			}
		})
	}
}

// Files directly under scenarios/ are the catalogue's own, such as the
// red-by-design list, and no scenario.
func TestLocateAllRunsACleanTreeBesideTheCataloguesOwnFiles(t *testing.T) {
	root := scenarioTree(t,
		"scenarios/red-by-design.txt",
		"scenarios/README.md",
		"scenarios/flow/flow-02.yaml",
		"scenarios/auth/auth-01.yaml",
		"scenarios/auth/.DS_Store",
	)
	found, err := locate(root, settings{all: true})
	if err != nil {
		t.Fatalf("locate: %v", err)
	}
	want := []string{
		filepath.Join(root, "scenarios/auth/auth-01.yaml"),
		filepath.Join(root, "scenarios/flow/flow-02.yaml"),
	}
	if len(found) != len(want) || found[0] != want[0] || found[1] != want[1] {
		t.Errorf("found %v, want %v", found, want)
	}
}

// A suite directory that moved away and left its link behind is a suite -all
// would pass over.
func TestLocateAllRefusesASuiteThatIsNotThere(t *testing.T) {
	root := scenarioTree(t, "scenarios/flow/flow-02.yaml")
	lost := filepath.Join(root, "scenarios", "auth")
	if err := os.Symlink(filepath.Join(root, "moved"), lost); err != nil {
		t.Fatal(err)
	}
	found, err := locate(root, settings{all: true})
	if err == nil || !strings.Contains(err.Error(), lost) {
		t.Errorf("-all ran %v beside a suite that is not there: %v", found, err)
	}
}
