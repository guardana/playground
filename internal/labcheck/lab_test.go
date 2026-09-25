// Package labcheck holds no code. It is the guard that every trajectory and
// scenario file in this repository loads and cross-validates, so a file that
// only a person has read cannot reach a run.
//
// It is a test rather than a script because the rules it enforces are the ones
// internal/labspec already states, and a second implementation of them in shell
// would be a second opinion about what a scenario means.
package labcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

const repoRoot = "../.."

// A scenario names its trajectory as a path from the repository root.
func TestEveryScenarioLoadsAndMatchesItsTrajectory(t *testing.T) {
	scenarios := filesUnder(t, "scenarios")
	if len(scenarios) == 0 {
		// A count of zero is a finding, not a pass: this guard exists to look
		// at scenario files, and a run of it that looked at none proves
		// nothing about the ones somebody will add tomorrow.
		t.Fatal("no scenarios found; this guard inspected nothing")
	}
	for _, path := range scenarios {
		t.Run(relative(path), func(t *testing.T) {
			scenario, err := labspec.LoadScenario(path)
			if err != nil {
				t.Fatalf("LoadScenario: %v", err)
			}
			if scenario.IsVerifier() {
				// LoadScenario checks every rule a verifier scenario has; it
				// names no trajectory to agree with.
				return
			}
			trajectoryPath := filepath.Join(repoRoot, scenario.Trajectory)
			trajectory, err := labspec.LoadTrajectory(trajectoryPath)
			if err != nil {
				t.Fatalf("LoadTrajectory %s: %v", scenario.Trajectory, err)
			}
			if err := labspec.Validate(scenario, trajectory); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

// A trajectory nothing runs is a file that rots without anyone noticing.
func TestEveryTrajectoryLoadsAndIsUsed(t *testing.T) {
	trajectories := filesUnder(t, "trajectories")
	if len(trajectories) == 0 {
		t.Fatal("no trajectories found; this guard inspected nothing")
	}
	used := make(map[string]bool)
	for _, path := range filesUnder(t, "scenarios") {
		scenario, err := labspec.LoadScenario(path)
		if err != nil {
			// Reported by the test above; here it would only be noise.
			continue
		}
		used[filepath.Clean(filepath.Join(repoRoot, scenario.Trajectory))] = true
	}
	for _, path := range trajectories {
		t.Run(relative(path), func(t *testing.T) {
			if _, err := labspec.LoadTrajectory(path); err != nil {
				t.Fatalf("LoadTrajectory: %v", err)
			}
			if !used[filepath.Clean(path)] {
				t.Errorf("no scenario runs %s", relative(path))
			}
		})
	}
}

func filesUnder(t *testing.T, directory string) []string {
	t.Helper()
	var found []string
	root := filepath.Join(repoRoot, directory)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".yaml") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func relative(path string) string {
	trimmed, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	return trimmed
}
