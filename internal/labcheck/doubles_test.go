package labcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// The directories compose mounts as each double's scripts.
const (
	pdpScriptDir      = "config/pdp/"
	approverScriptDir = "config/approver/"
)

// A double started on a script that is not there never answers, so a scenario
// naming a missing one would go red for a reason nobody meant to test.
func TestEveryNamedDoubleScriptExists(t *testing.T) {
	named := 0
	for _, path := range filesUnder(t, "scenarios") {
		scenario, err := labspec.LoadScenario(path)
		if err != nil || !scenario.UsesEnforcer() {
			continue
		}
		for _, script := range []struct{ name, directory string }{
			{scenario.Gateway.PDPScript, pdpScriptDir},
			{scenario.Gateway.ApproverScript, approverScriptDir},
		} {
			if script.name == "" {
				continue
			}
			named++
			if _, err := os.Stat(filepath.Join(repoRoot, script.directory, script.name)); err != nil {
				t.Errorf("%s names script %s%s, which does not exist", relative(path), script.directory, script.name)
			}
		}
	}
	if named == 0 {
		t.Fatal("no scenario names a double's script; this guard inspected nothing")
	}
}
