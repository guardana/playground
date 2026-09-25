//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The approver writes its journal as its own uid and the runner reads it as
// another, so the mode must survive the strictest umask a host can set.
func TestTheJournalIsReadableByTheRunnerUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	reports := t.TempDir()
	runIn(t, newFakeLab(t, "mixed.txt"), 2*time.Second, reports)
	info, err := os.Stat(filepath.Join(reports, "run-7", "journals", "approver.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("journal is %o, want 644: readable by the runner, writable by the approver alone", got)
	}
}
