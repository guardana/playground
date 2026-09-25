//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The double writes its journal and its CA as its own uid; the runner reads
// the journal and the enforcer the CA, each as another uid, so both modes must
// survive the strictest umask a host can set.
func TestTheDoublesFilesAreReadableByAnotherUidUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	scriptPath := filepath.Join(t.TempDir(), "pdp.yaml")
	if err := os.WriteFile(scriptPath, []byte(scriptAnswering("allow")), 0o600); err != nil {
		t.Fatal(err)
	}
	srv := serveInProcess(t, scriptPath)
	waitForFile(t, srv.caPath)
	srv.cancel()
	select {
	case err := <-srv.done:
		if err != nil {
			t.Fatalf("serveOn returned %v on shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveOn did not return after its context ended")
	}
	journalPath := filepath.Join(srv.root, "reports", "run-e2e", "journals", "pdp-double.jsonl")
	for _, path := range []string{journalPath, srv.caPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Errorf("%s is %o, want 644: readable by another uid, writable by the double alone", filepath.Base(path), got)
		}
	}
}
