package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cat of an endless file such as /dev/zero would hold the container's memory
// until it died; a file one byte past the bound is refused instead, and one at
// the bound is read whole.
func TestCatReadsAFileOnlyUpToItsBound(t *testing.T) {
	dir := t.TempDir()
	at := filepath.Join(dir, "at-the-bound")
	past := filepath.Join(dir, "past-the-bound")
	if err := os.WriteFile(at, []byte(strings.Repeat("x", catLimit)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(past, []byte(strings.Repeat("x", catLimit+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := concatenate(context.Background(), []string{at}); got.exitCode != 0 || len(got.stdout) != catLimit {
		t.Errorf("a file at the bound: exit %d, %d bytes", got.exitCode, len(got.stdout))
	}
	got := concatenate(context.Background(), []string{past})
	if got.exitCode == 0 || got.stdout != "" || !strings.Contains(got.stderr, "more than") {
		t.Errorf("a file past the bound: exit %d, %d bytes out, stderr %q", got.exitCode, len(got.stdout), got.stderr)
	}
}

// The bound is on what one cat reads in all, and cat reads regular files only:
// a device or a pipe either never ends or never answers.
func TestCatReadsRegularFilesUpToOneBoundInAll(t *testing.T) {
	half := filepath.Join(t.TempDir(), "half")
	if err := os.WriteFile(half, []byte(strings.Repeat("x", catLimit/2+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := concatenate(context.Background(), []string{half, half}); got.exitCode == 0 || got.stdout != "" {
		t.Errorf("two files past the bound together: exit %d, %d bytes out", got.exitCode, len(got.stdout))
	}
	if got := concatenate(context.Background(), []string{os.DevNull}); got.exitCode == 0 || !strings.Contains(got.stderr, "not a regular file") {
		t.Errorf("a device was read: exit %d, stderr %q", got.exitCode, got.stderr)
	}
}
