package journal_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/journal"
)

var authAt = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// mixedJournal writes two calls and two auth decisions, interleaved, the way
// a token-gated victim does.
func mixedJournal(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "victim-vault.jsonl")
	w, err := journal.Open(path, "victim-vault")
	if err != nil {
		t.Fatal(err)
	}
	writes := []func() error{
		func() error {
			return w.RecordAuth(journal.Auth{OccurredAt: authAt, RunID: "run-1", Credential: journal.CredentialRun,
				Session: journal.SessionNone, Status: 200, Method: "initialize"})
		},
		func() error {
			return w.Record(journal.Entry{OccurredAt: authAt, RunID: "run-1", Tool: "vault.read", Status: journal.Served})
		},
		func() error {
			return w.RecordAuth(journal.Auth{OccurredAt: authAt, RunID: "run-1", Credential: journal.CredentialNone,
				Session: journal.SessionKnown, Status: 401})
		},
		func() error {
			return w.Record(journal.Entry{OccurredAt: authAt, RunID: "run-1", Tool: "vault.drop", Status: journal.Refused})
		},
	}
	for _, write := range writes {
		if err := write(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// An auth line counted as a call would add a served or refused call no tool
// made, or fail every effect grade on a status the lab does not know.
func TestAnAuthLineIsNeverACall(t *testing.T) {
	entries, err := journal.ReadFile(mixedJournal(t))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Tool+" "+string(entry.Status))
	}
	if want := []string{"vault.read served", "vault.drop refused"}; !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
	if counts := journal.CountsByTool(entries); !reflect.DeepEqual(counts, map[string]int{"vault.read": 1}) {
		t.Errorf("served = %v, want vault.read=1", counts)
	}
}

func TestAJournalMixingKindsRoundTrips(t *testing.T) {
	lines, err := journal.ReadAuthFile(mixedJournal(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []journal.Auth{
		{OccurredAt: authAt, Server: "victim-vault", RunID: "run-1", Credential: "run", Session: "none", Status: 200, Method: "initialize"},
		{OccurredAt: authAt, Server: "victim-vault", RunID: "run-1", Credential: "none", Session: "known", Status: 401},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("auth lines = %+v, want %+v", lines, want)
	}
}

// versionOneEntry is a call line as a reader of version 1 decodes it, written
// out here rather than taken from the package, which has moved on.
type versionOneEntry struct {
	OccurredAt time.Time `json:"occurred_at"`
	Server     string    `json:"server"`
	Tool       string    `json:"tool"`
	RunID      string    `json:"run_id,omitempty"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	Effect     any       `json:"effect,omitempty"`
}

// A reader of version 1 decodes every line as a call and ignores fields it
// does not know, so an auth line has to be one it cannot decode at all.
func TestAVersionOneReaderRefusesAnAuthLine(t *testing.T) {
	body, err := os.ReadFile(mixedJournal(t))
	if err != nil {
		t.Fatal(err)
	}
	var refused, read int
	for _, line := range bytes.Split(bytes.TrimSpace(body), []byte("\n")) {
		var entry versionOneEntry
		if json.Unmarshal(line, &entry) != nil {
			refused++
			continue
		}
		read++
	}
	if refused != 2 || read != 2 {
		t.Errorf("a version 1 reader refused %d line(s) and read %d, want 2 and 2", refused, read)
	}
}

func TestAJournalWithoutAVersionStillReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "victim-fs.jsonl")
	body := `{"occurred_at":"2026-01-01T00:00:00Z","server":"victim-fs","tool":"fs.read","run_id":"run-1","status":"served"}` + "\n" +
		`{"occurred_at":"2026-01-01T00:00:01Z","server":"victim-fs","tool":"fs.write","status":"refused","detail":"outside"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := journal.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Tool != "fs.read" || entries[1].Status != journal.Refused {
		t.Errorf("entries = %+v", entries)
	}
	auth, err := journal.ReadAuthFile(path)
	if err != nil || len(auth) != 0 {
		t.Errorf("auth lines = %+v, %v; want none and no error", auth, err)
	}
}

// The run's token is 32 random bytes; any field that could hold it is refused,
// and the refusal does not repeat it either.
func TestRecordAuthRefusesAnythingThatCouldHoldASecret(t *testing.T) {
	secret := strings.Repeat("Zq9", 14) + "x"
	valid := journal.Auth{OccurredAt: authAt, Credential: journal.CredentialRun, Session: journal.SessionNone, Status: 200}
	tests := []struct {
		name   string
		change func(*journal.Auth)
	}{
		{"credential holding a token", func(a *journal.Auth) { a.Credential = journal.Credential("Bearer " + secret) }},
		{"session holding an id", func(a *journal.Auth) { a.Session = journal.Session(secret) }},
		{"method that is a token", func(a *journal.Auth) { a.Method = secret }},
		{"method that is a lowercase token", func(a *journal.Auth) { a.Method = strings.ToLower(strings.ReplaceAll(secret, "9", "")) }},
		{"method carrying a token", func(a *journal.Auth) { a.Method = "tools/call?token=" + secret }},
		{"method with a space", func(a *journal.Auth) { a.Method = "tools/call " + secret }},
		{"method past the length bound", func(a *journal.Auth) {
			a.Method = strings.Repeat(strings.Repeat("q", 24)+"/", 2) + strings.Repeat("q", 24)
		}},
		{"no time", func(a *journal.Auth) { a.OccurredAt = time.Time{} }},
		{"no status", func(a *journal.Auth) { a.Status = 0 }},
		{"status past HTTP", func(a *journal.Auth) { a.Status = 600 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "victim-vault.jsonl")
			w, err := journal.Open(path, "victim-vault")
			if err != nil {
				t.Fatal(err)
			}
			line := valid
			test.change(&line)
			err = w.RecordAuth(line)
			if closeErr := w.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if !errors.Is(err, journal.ErrInvalidEntry) {
				t.Fatalf("RecordAuth = %v, want ErrInvalidEntry", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Error("the refusal repeats the value it refused")
			}
			if body, _ := os.ReadFile(path); len(body) != 0 {
				t.Errorf("a refused line reached the journal: %s", body)
			}
		})
	}
}

func TestMCPMethodNamesAreValid(t *testing.T) {
	for _, method := range []string{"initialize", "tools/call", "notifications/initialized",
		"resources/templates/list", "notifications/roots/list_changed", "logging/setLevel"} {
		if !journal.ValidMethod(method) {
			t.Errorf("%q is refused as a method name", method)
		}
	}
}

// A line this reader cannot place is refused by both readers, never skipped:
// a reader that skipped it could not tell a newer call line from noise.
func TestTheReaderRefusesALineItDoesNotKnow(t *testing.T) {
	const at = `"occurred_at":"2026-01-01T00:00:00Z","server":"victim-vault"`
	tests := map[string]string{
		"a newer version":            `{"version":3,` + at + `,"tool":"vault.read","status":"served"}`,
		"an unknown kind":            `{"version":2,"kind":"grant",` + at + `,"status":200}`,
		"an auth line of version 1":  `{"kind":"auth",` + at + `,"credential":"run","session":"none","status":200}`,
		"an auth field unknown":      `{"version":2,"kind":"auth",` + at + `,"credential":"run","session":"none","status":200,"token":"x"}`,
		"an auth credential unknown": `{"version":2,"kind":"auth",` + at + `,"credential":"bearer","session":"none","status":200}`,
		"an auth line with no time":  `{"version":2,"kind":"auth","server":"victim-vault","credential":"run","session":"none","status":200}`,
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "victim-vault.jsonl")
			call := `{` + at + `,"tool":"vault.read","status":"served"}`
			if err := os.WriteFile(path, []byte(call+"\n"+line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if entries, err := journal.ReadFile(path); err == nil || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("ReadFile = %d entries, %v; want line 2 refused", len(entries), err)
			}
			if lines, err := journal.ReadAuthFile(path); err == nil || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("ReadAuthFile = %d lines, %v; want line 2 refused", len(lines), err)
			}
		})
	}
}
