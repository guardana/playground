//go:build unix

package mcpserve_test

import (
	"os"
	"syscall"
	"testing"

	"github.com/guardana/playground/victims/mcpserve"
)

// Every victim writes its journal as its own uid and the runner reads it as
// another, so the mode must survive the strictest umask a host can set.
func TestAVictimsJournalIsReadableByTheRunnerUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	config := configIn(t.TempDir())
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(config.JournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("journal is %o, want 644: readable by the runner, writable by the victim alone", got)
	}
}
