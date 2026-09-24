package labcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// A contract the verifier cannot open is refused with exit 3, so a scenario
// naming a missing one would go red for a reason nobody meant to test.
func TestEveryNamedContractExists(t *testing.T) {
	scenarios := filesUnder(t, "scenarios")
	if len(scenarios) == 0 {
		t.Fatal("no scenarios found; this guard inspected nothing")
	}
	for _, path := range scenarios {
		scenario, err := labspec.LoadScenario(path)
		if err != nil || scenario.Trace == nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, scenario.Trace.Contract)); err != nil {
			t.Errorf("%s names contract %s, which does not exist", relative(path), scenario.Trace.Contract)
		}
	}
}
