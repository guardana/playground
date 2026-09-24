package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

func TestEveryRunGetsItsOwnComposeProject(t *testing.T) {
	compose := workingCompose("")
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	var projects []string
	for range 2 {
		if _, err := subject.execute(context.Background(), scenario); err != nil {
			t.Fatalf("execute: %v", err)
		}
		projects = append(projects, compose.env["COMPOSE_PROJECT_NAME"])
	}
	if projects[0] != "lab-run1" || projects[1] != "lab-run2" {
		t.Errorf("projects = %q, want lab-run1 and lab-run2", projects)
	}
}

func TestTheRunIsGradedOnTheTrailsItOpened(t *testing.T) {
	compose := workingCompose("")
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, result := range graded.Results {
		if result.Check == "trails/opened" {
			if result.Outcome != assertion.Pass {
				t.Errorf("trails/opened is %s: %s", result.Outcome, result.Detail)
			}
			return
		}
	}
	t.Error("the run was not graded on the trails it opened")
}

func TestAKnownGapIsNamedInTheReport(t *testing.T) {
	compose := workingCompose("")
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	gaps := filepath.Join(subject.root, "scenarios", "gaps")
	body := scenarioFile + "gap: { wanted: { 1: { verdict: DENY } }, why: the plane builds no run flow }\n"
	writeFile(filepath.Join(gaps, "flow-01.yaml"), body)
	if err := os.Remove(scenario); err != nil {
		t.Fatal(err)
	}
	graded, err := subject.execute(context.Background(), filepath.Join(gaps, "flow-01.yaml"))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var out strings.Builder
	if err := executeAll(context.Background(), subject, []string{filepath.Join(gaps, "flow-01.yaml")}, &out); err != nil {
		t.Fatalf("executeAll: %v", err)
	}
	if !strings.Contains(out.String(), "known-gap") {
		t.Errorf("the summary line does not name the gap suite: %q", out.String())
	}
	if graded.Gap != "the plane builds no run flow" {
		t.Fatalf("the graded run names gap %q", graded.Gap)
	}
	page, err := os.ReadFile(filepath.Join(subject.reports, graded.RunID, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Known gap: the plane builds no run flow", "| Wanted |", "| DENY |"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("report.md does not say %q:\n%s", want, page)
		}
	}
}

// A scenario without a stub runs against the enforcer, which stamps no run id:
// its trail is this run's because it was read from the directory the run made.
func TestAScenarioWithoutAStubIsGradedOnATrailThatNamesNoRun(t *testing.T) {
	compose := workingCompose("")
	compose.trail = strings.NewReplacer(`"runId":"${RUN_ID}",`, "", `"runId":"${RUN_ID}"`, "").Replace(evidenceFile)
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	writeFile(scenario, strings.Replace(scenarioFile, "stub: { verdicts: config/scenarios/flow-01.yaml }\n", "", 1))
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if compose.env["LAB_STUB_VERDICTS"] != "" {
		t.Fatalf("a scenario without a stub handed the stub verdicts %q", compose.env["LAB_STUB_VERDICTS"])
	}
	for _, result := range graded.Results {
		if result.Check == "evidence/run-id" {
			if result.Outcome != assertion.Pass || !strings.Contains(result.Want, "no event names a run") {
				t.Errorf("evidence/run-id is %s against %q: %s", result.Outcome, result.Want, result.Detail)
			}
			return
		}
	}
	t.Error("the run was not graded on which run wrote its trail")
}

func TestTheAgentIsToldTheEnforcersNamespace(t *testing.T) {
	compose := workingCompose("")
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	replay := compose.ran[len(compose.ran)-1]
	if !strings.Contains(strings.Join(replay, " "), "-namespace guardana.control") {
		t.Errorf("the replay ran with %q", replay)
	}
}

func TestTheNamespaceIsReadFromThePins(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=abc\nENFORCER_NAMESPACE=guardana.control\n")
	if got, err := enforcerNamespace(root); err != nil || got != "guardana.control" {
		t.Errorf("enforcerNamespace = %q, %v", got, err)
	}
	writeFile(filepath.Join(root, versionFile), "ENFORCER_COMMIT=abc\n")
	if got, err := enforcerNamespace(root); err == nil {
		t.Errorf("a versions.env without the namespace gave %q", got)
	}
}
