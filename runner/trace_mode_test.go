package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// The verifier reads the trace as another uid, so a host umask of 077 must not
// narrow what the runner hands it.
func TestTheTraceIsReadableByTheVerifierUnderAStrictUmask(t *testing.T) {
	previous := syscall.Umask(0o077)
	defer syscall.Umask(previous)
	dir := t.TempDir()
	from := filepath.Join(dir, "trace.jsonl")
	writeFile(from, `{"guardana_trace":3}`+"\n")
	to := filepath.Join(dir, "verifier")
	if err := copyTrace(from, to); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{to: 0o755, filepath.Join(to, traceFile): 0o644} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s is %o, want %o", filepath.Base(path), got, want)
		}
	}
}
