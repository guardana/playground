package labcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// Every profile a verifier step names exists, and every profile in the
// directory is named by a step: one nobody hands the verifier is a profile
// nothing tests.
func TestEveryVerifierProfileIsNamedAndExists(t *testing.T) {
	scenarios := filesUnder(t, "scenarios")
	if len(scenarios) == 0 {
		t.Fatal("no scenarios found; this guard inspected nothing")
	}
	var named []string
	for _, path := range scenarios {
		scenario, err := labspec.LoadScenario(path)
		if err != nil {
			continue
		}
		for _, step := range scenario.Verifier {
			profile := ""
			switch {
			case step.Probe != nil:
				profile = step.Probe.Profile
			case step.Scan != nil:
				profile = step.Scan.Profile
			}
			if profile == "" {
				continue
			}
			named = append(named, profile)
			if _, err := os.Stat(filepath.Join(repoRoot, profile)); err != nil {
				t.Errorf("%s names profile %s, which does not exist", relative(path), profile)
			}
		}
	}
	for _, path := range filesUnder(t, labspec.ProfileDir) {
		if profile := relative(path); !slices.Contains(named, profile) {
			t.Errorf("%s is named by no verifier step", profile)
		}
	}
}
