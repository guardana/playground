package journal_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

func TestWriterRecordsWhatTheServerServed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	w, err := journal.Open(path, "victim-fs")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	if err := w.Record(journal.Entry{
		OccurredAt: at, Tool: "fs.read", RunID: "run-1", Status: journal.Served, Detail: "/data/private/customers.csv",
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Server != "victim-fs" {
		t.Errorf("server = %q, want victim-fs; the writer stamps it so a caller cannot mislabel it", entries[0].Server)
	}
	if entries[0].Tool != "fs.read" || entries[0].Status != journal.Served {
		t.Errorf("entry = %+v", entries[0])
	}
}

// The journal is the record a scenario grades effects from. A tool name it
// cannot attribute is a line that would let a call go uncounted.
func TestRecordRefusesAnEntryWithNoTool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	w, err := journal.Open(path, "victim-fs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if err := w.Record(journal.Entry{OccurredAt: time.Now(), Status: journal.Served}); err == nil {
		t.Fatal("an entry naming no tool was accepted")
	}
}

func TestRecordRefusesAnUnknownStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	w, err := journal.Open(path, "victim-fs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()
	if err := w.Record(journal.Entry{OccurredAt: time.Now(), Tool: "fs.read", Status: "maybe"}); err == nil {
		t.Fatal("an unknown status was accepted")
	}
}

// A tool server handles calls concurrently. Two lines interleaved into one is a
// journal that undercounts, which is the direction that hides a call.
func TestWriterIsSafeForConcurrentUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	w, err := journal.Open(path, "victim-fs")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Record(journal.Entry{OccurredAt: time.Now(), Tool: "fs.read", Status: journal.Served}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 64 {
		t.Errorf("entries = %d, want 64", len(entries))
	}
}

// A victim that served nothing writes a file with no lines. Reading it as an
// empty journal is right; reading a missing file as one is not, because a
// server that never started would then look like a server that refused.
func TestReadFileTellsEmptyFromAbsent(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "victim-mail.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.ReadFile(empty)
	if err != nil {
		t.Fatalf("ReadFile on an empty journal: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
	if _, err := journal.ReadFile(filepath.Join(dir, "victim-db.jsonl")); err == nil {
		t.Fatal("a journal that does not exist read as an empty one")
	}
}

func TestReadFileRefusesAMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	if err := os.WriteFile(path, []byte("{not json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.ReadFile(path); err == nil {
		t.Fatal("a malformed line was read as a journal")
	}
}

func TestCountsByToolCountsOnlyWhatWasServed(t *testing.T) {
	entries := []journal.Entry{
		{Tool: "fs.read", Status: journal.Served},
		{Tool: "fs.read", Status: journal.Served},
		{Tool: "fs.write", Status: journal.Refused},
	}
	counts := journal.CountsByTool(entries)
	if counts["fs.read"] != 2 {
		t.Errorf("fs.read = %d, want 2", counts["fs.read"])
	}
	if _, present := counts["fs.write"]; present {
		t.Error("a refused call was counted as served")
	}
}

func TestEntryLinesAreOneLineEach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	w, err := journal.Open(path, "victim-fs")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Record(journal.Entry{
		OccurredAt: time.Now(), Tool: "fs.read", Status: journal.Served, Detail: "a\nb",
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "\n") != 1 {
		t.Errorf("one entry wrote %d lines: %q", strings.Count(string(body), "\n"), body)
	}
}

// A server that writes past the limit makes its whole journal unreadable, and
// an effect assertion reading no journal establishes nothing. The refusal has
// to name the line and the limit, because the reader's next move is to open it.
func TestReadFileNamesAnOversizedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-db.jsonl")
	good := `{"occurred_at":"2026-09-09T10:00:00Z","server":"victim-db","tool":"db.query","status":"served"}` + "\n"
	huge := `{"occurred_at":"2026-09-09T10:00:01Z","server":"victim-db","tool":"db.execute",` +
		`"status":"served","detail":"` + strings.Repeat("x", journal.MaxLineBytes) + `"}` + "\n"
	if err := os.WriteFile(path, []byte(good+huge), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.ReadFile(path)
	if !errors.Is(err, journal.ErrLineTooLong) {
		t.Fatalf("err = %v, want ErrLineTooLong", err)
	}
	if entries != nil {
		t.Errorf("entries = %d, want none: a journal read short undercounts what the victim served", len(entries))
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("the refusal does not name the line: %v", err)
	}
}
