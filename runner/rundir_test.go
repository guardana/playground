package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The lab services run as nonroot, uid 65532, and write into the run directory
// the runner made for them: the stub gateway its evidence trail, each victim
// its journal. A directory the invoking user owns at 0750 is one none of them
// can write to on Linux, where the uid in the container is the uid on the
// bind mount. macOS hides it — Docker Desktop's file sharing maps every access
// to the invoking user — so the lab passes on a laptop and CI cannot write a
// single record.
func TestMakeRunDirIsWritableByTheServicesThatWriteIntoIt(t *testing.T) {
	reports := filepath.Join(t.TempDir(), "reports")
	runDir := filepath.Join(reports, "flow-01-20260909T120000Z-a1b2c3d4")
	if err := makeRunDir(reports, runDir); err != nil {
		t.Fatalf("makeRunDir: %v", err)
	}

	for _, directory := range []string{runDir, filepath.Join(runDir, "journals"), filepath.Join(runDir, "agent")} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode&0o007 != 0o007 {
			t.Errorf("%s is %04o, so a service running as uid 65532 can neither write nor list it",
				filepath.Base(directory), mode)
		}
	}
	// The runner is the only thing that writes into reports/ itself. A service
	// only has to be able to reach the run directory through it.
	info, err := os.Stat(reports)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o005 != 0o005 {
		t.Errorf("reports/ is %04o, so nothing in a container can reach the run directory through it", mode)
	}
}

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
