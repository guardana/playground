package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "/run/approvals"

// fixture is a listing as the enforcer's `approvals list` prints it at the
// pin in versions.env, with the directory it names put back in.
func fixture(t *testing.T, name, dir string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name)) // #nosec G304 -- the package's own fixtures.
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(body), "{{DIR}}", dir)
}

func TestParseListingReadsEveryRecordAndItsState(t *testing.T) {
	l, err := parseListing([]byte(fixture(t, "every-state.txt", fixtureDir)), fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if !l.planeHolds {
		t.Error("the listing says a plane holds the directory, parsed as none")
	}
	want := map[string]bool{"ANSWERED": false, "CLOSED": false, "SPENT": false, "STALE": true, "WAITING": true}
	if len(l.entries) != len(want) {
		t.Fatalf("entries = %d, want %d", len(l.entries), len(want))
	}
	for _, e := range l.entries {
		waiting, known := want[e.id]
		if !known {
			t.Fatalf("an entry %q the listing does not hold", e.id)
		}
		if e.waiting() != waiting {
			t.Errorf("%s: waiting = %v, want %v", e.id, e.waiting(), waiting)
		}
	}
	w := l.entries[4]
	got := [...]string{w.id, w.action, w.resource, w.effectClass, w.principal, w.agent}
	if got != [...]string{"WAITING", "refund", "payment pay-9", "EFFECT_CLASS_TRANSACT", "user-1", "agent-1"} {
		t.Errorf("WAITING read as %q", got)
	}
}

func TestParseListingReadsAnEmptyDirectoryAsNoRecords(t *testing.T) {
	l, err := parseListing([]byte(fixture(t, "empty.txt", fixtureDir)), fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.entries) != 0 {
		t.Errorf("entries = %d, want 0", len(l.entries))
	}
}

// A record whose projection will not decode still waits; it just has no
// readable field for a rule to match on.
func TestParseListingKeepsARecordWithNoReadableFields(t *testing.T) {
	l, err := parseListing([]byte(fixture(t, "broken-view.txt", fixtureDir)), fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	w := l.entries[4]
	if w.id != "WAITING" || !w.waiting() || w.action != "" || w.readable {
		t.Errorf("WAITING read as %+v", w)
	}
}

// Every one of these is a listing the approver cannot read, and reading any of
// them as "nothing waits" would let a held call expire unanswered unnoticed.
func TestParseListingRefusesWhatItCannotRead(t *testing.T) {
	good := fixture(t, "every-state.txt", fixtureDir)
	firstBlock := strings.Index(good, "\napproval ANSWERED")
	cases := map[string]string{
		"nothing":                 "",
		"garbage":                 "{not a listing}\n",
		"no final newline":        strings.TrimSuffix(good, "\n"),
		"another directory":       fixture(t, "every-state.txt", "/elsewhere"),
		"a count it does not say": strings.Replace(good, "5 records", "five records", 1),
		"fewer records than said": good[:firstBlock] + good[strings.Index(good, "\napproval CLOSED"):],
		"more records than said":  strings.Replace(good, "5 records", "4 records", 1),
		"an unknown plane line":   strings.Replace(good, "a plane holds this directory", "a plane may hold it", 1),
		"a changed disclaimer":    strings.Replace(good, "which no record holds", "which nobody holds", 1),
		"an unknown field":        strings.Replace(good, "  agent  ", "  agnet  ", 1),
		"a field twice":           strings.Replace(good, "  agent          agent-1\n", "  agent          agent-1\n  agent          agent-2\n", 1),
		"a record with no state":  strings.Replace(good, "  state          APPROVAL_STATE_APPROVED\n", "", 1),
		"a field with no value":   strings.Replace(good, "  request        req-answered\n", "  request\n", 1),
		"a field outside records": good[:firstBlock] + "\n  state          APPROVAL_STATE_PENDING" + good[firstBlock:],
		"no blank line between":   strings.Replace(good, "\n\napproval CLOSED", "\napproval CLOSED", 1),
		"a nameless approval":     strings.Replace(good, "approval CLOSED\n", "approval \n", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if body == good {
				t.Fatal("the mutation changed nothing, so this case examines nothing")
			}
			if _, err := parseListing([]byte(body), fixtureDir); !errors.Is(err, errListing) {
				t.Errorf("err = %v, want errListing", err)
			}
		})
	}
}
