package journal_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

// A victim writes its journal into a directory it can write, so what sits at
// the journal's name is the victim's to choose.
func TestAJournalThatIsALinkIsRefusedWithoutReadingItsTarget(t *testing.T) {
	dir := t.TempDir()
	const canary = "host-file-canary-7f3a"
	target := filepath.Join(t.TempDir(), "host-file")
	line := `{"occurred_at":"2026-01-01T00:00:00Z","server":"victim-fs","tool":"read_file","status":"served","detail":"` + canary + `"}` + "\n"
	if err := os.WriteFile(target, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "victim-fs.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.ReadFile(path)
	if err == nil {
		t.Fatalf("a journal that is a link was read: %+v", entries)
	}
	if strings.Contains(err.Error(), canary) {
		t.Errorf("the refusal carries the link target's bytes: %v", err)
	}
}

func TestAJournalThatIsAFIFOIsRefusedWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := journal.ReadFile(path)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a journal that is a FIFO was read")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reading a journal that is a FIFO blocked")
	}
}

// A journal past the bound is refused whole, never read short.
func TestAJournalPastTheFileBoundIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	line := []byte(`{"occurred_at":"2026-01-01T00:00:00Z","server":"victim-fs","tool":"read_file","status":"served","detail":"` +
		strings.Repeat("x", 4000) + `"}` + "\n")
	body := bytes.Repeat(line, journal.MaxFileBytes/len(line)+1)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.ReadFile(path); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("a journal of %d bytes was %v, want refused as past the bound", len(body), err)
	}
	within := body[:len(body)-len(line)]
	if err := os.WriteFile(path, within, 0o600); err != nil {
		t.Fatal(err)
	}
	if entries, err := journal.ReadFile(path); err != nil || len(entries) != len(within)/len(line) {
		t.Errorf("a journal within the bound read %d entries, %v", len(entries), err)
	}
}
