package labcheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// Every target a scan step names is a directory of the tree, and every
// variant under artifacts/ is scanned by one: an artifact nobody scans is a
// defect nothing tests.
func TestEveryArtifactIsScannedAndEveryScanTargetExists(t *testing.T) {
	var named []string
	for _, path := range filesUnder(t, "scenarios") {
		scenario, err := labspec.LoadScenario(path)
		if err != nil {
			continue
		}
		for _, step := range scenario.Verifier {
			if step.Scan == nil {
				continue
			}
			named = append(named, step.Scan.Target)
			if info, err := os.Stat(filepath.Join(repoRoot, step.Scan.Target)); err != nil || !info.IsDir() {
				t.Errorf("%s scans %s, which is not a directory of the tree", relative(path), step.Scan.Target)
			}
		}
	}
	cases, err := os.ReadDir(filepath.Join(repoRoot, labspec.ArtifactDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range cases {
		if !artifact.IsDir() {
			continue
		}
		for _, variant := range []string{"control", "defect"} {
			target := labspec.ArtifactDir + artifact.Name() + "/" + variant
			if !slices.Contains(named, target) {
				t.Errorf("%s is scanned by no scenario", target)
			}
		}
	}
}
