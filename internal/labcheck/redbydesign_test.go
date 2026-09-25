package labcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/guardana/playground/internal/redbydesign"
)

// redByDesignList is the list CI judges the catalogue against.
const redByDesignList = "scenarios/red-by-design.txt"

// A listed identifier no scenario carries is caught here, before a catalogue
// run of many minutes reports the same.
func TestEveryScenarioListedRedByDesignExists(t *testing.T) {
	listed, err := redbydesign.Read(filepath.Join(repoRoot, redByDesignList))
	if err != nil {
		t.Fatalf("%s: %v", redByDesignList, err)
	}
	for _, entry := range listed {
		found, err := filepath.Glob(filepath.Join(repoRoot, "scenarios", "*", entry.ID+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 {
			t.Errorf("%s lists %s, and %d scenario files carry that identifier", redByDesignList, entry.ID, len(found))
		}
	}
}
