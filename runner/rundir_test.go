package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// Two runs of one scenario are two runs, and a run that wrote into a directory
// that was already there would be graded on what the last one left in it.
func TestMakeRunDirRefusesADirectoryThatIsAlreadyThere(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runDir := filepath.Join(reports, "flow-01-20260909T120000Z-a1b2c3d4")
	if err := makeRunDir(reports, runDir); err != nil {
		t.Fatalf("makeRunDir: %v", err)
	}
	if err := makeRunDir(reports, runDir); err == nil {
		t.Error("a second run was handed the first run's directory")
	}
}

// The agent sees its own directory of the run and nothing else of it, so its
// log goes there.
func TestTheAgentWritesIntoItsOwnDirectory(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	replay := strings.Join(compose.ran[len(compose.ran)-1], " ")
	if !strings.Contains(replay, "-out /reports/"+compose.env["LAB_RUN_ID"]+"/agent/agent.jsonl") {
		t.Errorf("the agent was told to write elsewhere: %s", replay)
	}
}
