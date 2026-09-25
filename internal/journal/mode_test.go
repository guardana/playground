//go:build unix

package journal_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/guardana/playground/internal/journal"
)

// A server writes its journal as its own uid and the runner reads it as
// another, so the mode must survive the strictest umask a host can set.
func TestAJournalIsReadableByAnotherUidUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	path := filepath.Join(t.TempDir(), "victim-crm.jsonl")
	w, err := journal.Open(path, "victim-crm")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("journal is %o, want 644: readable by the runner, writable by its server alone", got)
	}
}
