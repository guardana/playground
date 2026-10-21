package journal_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// The runner reads both kinds from the same file, and the directory check
// holds for each: an auth line read from beside an intruder is no record.
func TestReadServerSplitsTheKindsOfOneJournal(t *testing.T) {
	dir := ownDirectory(t, "victim-vault")
	writeOneOfEach(t, journal.File(dir, "victim-vault"))

	calls, err := journal.ReadServer(dir, "victim-vault")
	if err != nil || len(calls) != 1 || calls[0].Tool != "vault.read" {
		t.Errorf("calls = %+v, %v; want vault.read alone", calls, err)
	}
	auth, err := journal.ReadServerAuth(dir, "victim-vault")
	if err != nil || len(auth) != 1 || auth[0].Credential != "other" || auth[0].Session != "unknown" || auth[0].Status != 401 {
		t.Errorf("auth lines = %+v, %v; want one other/unknown/401", auth, err)
	}

	if err := os.WriteFile(filepath.Join(dir, "victim-vault", "other.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if auth, err := journal.ReadServerAuth(dir, "victim-vault"); err == nil {
		t.Errorf("read %+v from a directory holding another file", auth)
	}
}

func writeOneOfEach(t *testing.T, path string) {
	t.Helper()
	w, err := journal.Open(path, "victim-vault")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	if err := errors.Join(
		w.RecordAuth(journal.Auth{OccurredAt: at, Credential: journal.CredentialOther, Session: journal.SessionUnknown, Status: 401}),
		w.Record(journal.Entry{OccurredAt: at, Tool: "vault.read", Status: journal.Served}),
		w.Close(),
	); err != nil {
		t.Fatal(err)
	}
}
