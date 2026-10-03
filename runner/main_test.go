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

func TestParseRefusesAnArgumentThatIsNoFlag(t *testing.T) {
	var out strings.Builder
	if got, err := parse([]string{"-scenario", "flow-01", "-reports", "/tmp/lab", "runs"}, &out); err == nil {
		t.Fatalf("parsed %+v; a path split at its space must be refused", got)
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
	subject, compose, scenario := enforcerLab(t)

	var out strings.Builder
	if err := executeAll(context.Background(), subject, []string{scenario}, &out); err != nil {
		t.Fatalf("a run that passed reported %v", err)
	}
	if !strings.Contains(out.String(), "pass") || !strings.Contains(out.String(), "flow-01") {
		t.Errorf("the summary is %q", out.String())
	}

	compose.collector = ""
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
	subject, _, _ := enforcerLab(t)
	missing := filepath.Join(subject.root, "scenarios/flow/absent.yaml")
	writeFile(missing, "schema_version: 2\n")

	var out strings.Builder
	if err := executeAll(context.Background(), subject, []string{missing}, &out); err == nil {
		t.Error("a scenario that does not load was reported as a run that passed")
	}
}

// An empty development image is a mistake upstream of the runner, such as a
// build whose name was not captured; read as "no development build" it would
// grade the pinned release under a development run's name.
func TestParseRefusesADevelopmentImageGivenEmpty(t *testing.T) {
	var out strings.Builder
	if _, err := parse([]string{"-all", "-enforcer-dev", ""}, &out); err == nil {
		t.Error("an empty -enforcer-dev was taken")
	}
	if got, err := parse([]string{"-all"}, &out); err != nil || got.enforcerDev != "" {
		t.Errorf("a run without -enforcer-dev: %+v, %v", got, err)
	}
}
