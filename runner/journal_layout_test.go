package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// Each writer's journal lives in a directory of its own, the only one compose
// mounts into it; the runner reads it there and names that file in the report.
func TestAJournalIsReadFromItsWritersOwnDirectory(t *testing.T) {
	subject, compose, scenario := verifierUnderTest(t)
	working := compose.split
	compose.split = func(service, entrypoint string, args []string) (Split, error) {
		split, err := working(service, entrypoint, args)
		runDir := compose.env["LAB_RUN_HOST_DIR"]
		writeFile(filepath.Join(runDir, "journals/victim-crm/victim-crm.jsonl"),
			`{"occurred_at":"2026-09-09T12:00:03Z","server":"victim-crm","tool":"crm.delete_customer","run_id":"`+
				compose.env["LAB_RUN_ID"]+`","status":"served"}`+"\n")
		return split, err
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	crm := results(graded)["effects/victim-crm"]
	if crm.Outcome != assertion.Fail || !strings.Contains(crm.Got, "crm.delete_customer=1") {
		t.Errorf("effects/victim-crm = %s %q, want the served call read from the journal", crm.Outcome, crm.Got)
	}
	if want := compose.env["LAB_RUN_HOST_DIR"] + "/journals/victim-crm/victim-crm.jsonl"; crm.Source != want {
		t.Errorf("effects/victim-crm names %s, want %s", crm.Source, want)
	}
}

// A writer's directory holds its journal alone. Anything beside it was put
// there by something other than the journal writer, so the journal is not
// graded and the run says what it found.
func TestAJournalDirectoryHoldingAnythingElseIsNotGraded(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	var log bytes.Buffer
	subject.log = &log
	compose.planted = map[string]string{"journals/victim-fs/other.jsonl": ""}

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fs := results(graded)["effects/victim-fs"]; fs.Outcome != assertion.Fail || fs.Got != "journal unreadable" || !strings.Contains(fs.Detail, "other.jsonl") {
		t.Errorf("effects/victim-fs = %s %q %q, want a journal that was not read, naming what was beside it", fs.Outcome, fs.Got, fs.Detail)
	}
	if graded.Outcome() == assertion.Pass {
		t.Error("a run whose journal directory held another file reported pass")
	}
	if !strings.Contains(log.String(), "other.jsonl") {
		t.Errorf("the run does not name the file it found:\n%s", log.String())
	}
}

// A journal that is not there is the victim that never wrote one, which the
// report keeps apart from one that is there and was refused.
func TestAMissingJournalStaysNoJournal(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	delete(compose.journals, "victim-fs")
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fs := results(graded)["effects/victim-fs"]; fs.Outcome != assertion.Fail || fs.Got != "no journal" {
		t.Errorf("effects/victim-fs = %s %q, want a missing journal", fs.Outcome, fs.Got)
	}
}
