package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDefaultsToTheReportsDirectoryTheMakefileUses(t *testing.T) {
	var out strings.Builder
	got, err := parse([]string{"-scenario", "flow-01"}, &out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.reports != "reports" {
		t.Errorf("-reports defaults to %q, want reports", got.reports)
	}
	if got.keep || got.all {
		t.Errorf("parsed %+v, want -keep and -all off", got)
	}
	// Every docker call the runner makes is bounded by this, so a scenario that
	// wedges is a red run rather than a build that never returns.
	if got.timeout != defaultScenarioTimeout {
		t.Errorf("-timeout defaults to %s, want %s", got.timeout, defaultScenarioTimeout)
	}
}

func TestParseTakesATimeoutForASlowerMachine(t *testing.T) {
	var out strings.Builder
	got, err := parse([]string{"-scenario", "flow-01", "-timeout", "45m"}, &out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.timeout != 45*time.Minute {
		t.Errorf("-timeout parsed as %s, want 45m", got.timeout)
	}
}

func TestExecuteAllReportsEveryScenarioAndFailsIfAnyDid(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports

	var out strings.Builder
	if err := executeAll(context.Background(), subject, []string{scenario}, &out); err != nil {
		t.Fatalf("a run that passed reported %v", err)
	}
	if !strings.Contains(out.String(), "pass") || !strings.Contains(out.String(), "flow-01") {
		t.Errorf("the summary is %q", out.String())
	}

	compose.trail = ""
	out.Reset()
	err := executeAll(context.Background(), subject, []string{scenario}, &out)
	if err == nil {
		t.Fatal("a run with no evidence trail reported success to the caller")
	}
	if !strings.Contains(err.Error(), "flow-01") {
		t.Errorf("the failure does not name the scenario: %v", err)
	}
}

// A scenario that could not even be read is a red run and not a skipped one.
func TestExecuteAllFailsOnAScenarioItCannotLoad(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, _ := labUnderTest(t, compose)
	compose.reports = subject.reports
	missing := filepath.Join(subject.root, "scenarios/flow/absent.yaml")
	writeFile(missing, "schema_version: 2\n")

	var out strings.Builder
	if err := executeAll(context.Background(), subject, []string{missing}, &out); err == nil {
		t.Error("a scenario that does not load was reported as a run that passed")
	}
}
