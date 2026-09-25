package main

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
)

// The agent sees trajectories/ mounted at its own name, so a repository path
// becomes a container path by putting a slash in front of it. A trajectory
// anywhere else would hand the agent a path that is not there.
func TestMountableRefusesATrajectoryTheAgentCannotSee(t *testing.T) {
	err := mountable(labspec.Scenario{Trajectory: "fixtures/flow.yaml"})
	if err == nil {
		t.Fatal("a trajectory the agent cannot see was accepted")
	}
	if !strings.Contains(err.Error(), "trajectory") {
		t.Errorf("error does not name the field: %v", err)
	}
}

func TestMountableAcceptsTheDirectoryComposeMounts(t *testing.T) {
	if err := mountable(labspec.Scenario{Trajectory: "trajectories/flow-01.yaml"}); err != nil {
		t.Fatalf("mountable: %v", err)
	}
}

// The translation and the rule have to agree, or the rule guards a path the
// runner does not build.
func TestInContainerMirrorsTheMountPoint(t *testing.T) {
	if got, want := inContainer("trajectories/flow-01.yaml"), "/trajectories/flow-01.yaml"; got != want {
		t.Errorf("inContainer = %q, want %q", got, want)
	}
}
