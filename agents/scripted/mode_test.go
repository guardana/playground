//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The agent writes its log and its trace as its own uid; the runner reads the
// trace as another, so the mode must survive the strictest umask a host can set.
func TestTheAgentsFilesAreReadableByAnotherUidUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	directory := filepath.Join(t.TempDir(), "agent")
	for _, name := range []string{"agent.jsonl", "trace.jsonl"} {
		path := filepath.Join(directory, name)
		file, err := createLog(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("%s is %o, want 644: readable by the runner, writable by the agent alone", name, got)
		}
	}
}
