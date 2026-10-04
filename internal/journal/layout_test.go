package journal_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/journal"
)

func TestAServersJournalSitsInADirectoryOfItsOwn(t *testing.T) {
	if got := journal.File("reports/run-1/journals", "victim-crm"); got != "reports/run-1/journals/victim-crm/victim-crm.jsonl" {
		t.Errorf("File = %s", got)
	}
}

func ownDirectory(t *testing.T, server string, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, server), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, server, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A server that never started left no journal, and one that served nothing
// left an empty one: the two stay apart when the journal has a directory.
func TestReadServerKeepsAMissingJournalApartFromAnEmptyOne(t *testing.T) {
	if _, err := journal.ReadServer(ownDirectory(t, "victim-fs"), "victim-fs"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a directory with no journal read as %v, want a missing journal", err)
	}
	entries, err := journal.ReadServer(ownDirectory(t, "victim-fs", "victim-fs.jsonl"), "victim-fs")
	if err != nil || len(entries) != 0 {
		t.Errorf("an empty journal read as %v, %v; want no entries and no error", entries, err)
	}
}

func TestReadServerRefusesADirectoryHoldingAnythingElse(t *testing.T) {
	for name, files := range map[string][]string{
		"another file beside the journal": {"victim-fs.jsonl", "other.jsonl"},
		"another file instead of it":      {"other.jsonl"},
		"another writer's journal":        {"victim-fs.jsonl", "victim-crm.jsonl"},
	} {
		t.Run(name, func(t *testing.T) {
			entries, err := journal.ReadServer(ownDirectory(t, "victim-fs", files...), "victim-fs")
			if err == nil {
				t.Fatalf("read %v from a directory holding %v", entries, files)
			}
			if intruder := files[len(files)-1]; !strings.Contains(err.Error(), intruder) {
				t.Errorf("the refusal %q does not name %s", err, intruder)
			}
		})
	}
}
