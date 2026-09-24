package main

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// The agent and the gateway see two directories, mounted at their own names, so
// a repository path becomes a container path by putting a slash in front of it.
// A scenario naming a file outside those two would boot a gateway pointed at a
// path that is not there, which is how the first end-to-end run of this lab
// failed: the gateway exited before it wrote a single evidence record, and the
// runner could only report that the trail was missing.
func TestMountableRefusesAFileTheContainersCannotSee(t *testing.T) {
	cases := []struct {
		name       string
		trajectory string
		verdicts   string
		wantField  string
	}{
		{
			name:       "a trajectory outside trajectories",
			trajectory: "fixtures/flow.yaml",
			verdicts:   "config/scenarios/flow.yaml",
			wantField:  "trajectory",
		},
		{
			name:       "declared verdicts outside config",
			trajectory: "trajectories/flow.yaml",
			verdicts:   "stubs/flow.yaml",
			wantField:  "stub.verdicts",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := mountable(labspec.Scenario{
				Trajectory: test.trajectory,
				Stub:       labspec.Stub{Verdicts: test.verdicts},
			})
			if err == nil {
				t.Fatal("a path the containers cannot see was accepted")
			}
			if !strings.Contains(err.Error(), test.wantField) {
				t.Errorf("error does not name the field: %v", err)
			}
		})
	}
}

func TestMountableAcceptsTheDirectoriesComposeMounts(t *testing.T) {
	err := mountable(labspec.Scenario{
		Trajectory: "trajectories/flow-01.yaml",
		Stub:       labspec.Stub{Verdicts: "config/scenarios/flow-01.yaml"},
	})
	if err != nil {
		t.Fatalf("mountable: %v", err)
	}
}

// A scenario with no stub is a scenario pointed at something that decides,
// which is where this lab is going. It must not be refused for a field it does
// not carry.
func TestMountableAcceptsAScenarioWithNoStub(t *testing.T) {
	if err := mountable(labspec.Scenario{Trajectory: "trajectories/flow-01.yaml"}); err != nil {
		t.Fatalf("mountable: %v", err)
	}
}

// The translation and the rule have to agree, or the rule guards a path the
// runner does not build.
func TestInContainerMirrorsTheMountPoints(t *testing.T) {
	for repositoryPath, want := range map[string]string{
		"trajectories/flow-01.yaml":     "/trajectories/flow-01.yaml",
		"config/scenarios/flow-01.yaml": "/config/scenarios/flow-01.yaml",
	} {
		if got := inContainer(repositoryPath); got != want {
			t.Errorf("inContainer(%q) = %q, want %q", repositoryPath, got, want)
		}
	}
}
