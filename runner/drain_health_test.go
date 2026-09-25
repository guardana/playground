package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// unrecordedReadHealth is drainedHealth with the pipeline counters of a read
// that ran unrecorded.
var unrecordedReadHealth = strings.Replace(drainedHealth, `{"status":"ok",`,
	`{"status":"ok","pipeline":{"blocks":{"EVIDENCE_UNAVAILABLE":1},"reads_unrecorded":1,"sink_failures_before_effect":2},`, 1)

// healthLab is a lab whose scenario states expect.health and whose enforcer
// answers /healthz with health, or cannot be read when health is empty.
func healthLab(t *testing.T, stated, health string) (lab, string) {
	t.Helper()
	subject, compose, scenario := enforcerLab(t)
	if stated != "" {
		body := strings.Replace(scenarioFile, "  evidence:\n", "  health: "+stated+"\n  evidence:\n", 1)
		writeFile(scenario, body)
	}
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if !strings.HasSuffix(args[len(args)-1], "/healthz") {
			return answer(service, args)
		}
		if health == "" {
			return Split{ExitCode: 1, Stderr: "connection refused"}
		}
		return Split{Stdout: "status 200\n" + health}
	}
	return subject, scenario
}

func TestTheHealthAnswerIsKeptAndGraded(t *testing.T) {
	subject, scenario := healthLab(t, "{ reads_unrecorded: 1, sink_failures_before_effect: 2 }", unrecordedReadHealth)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := results(graded)
	for _, name := range []string{"health/reads_unrecorded", "health/sink_failures_before_effect"} {
		if found[name].Outcome != assertion.Pass {
			t.Errorf("%s is %s: %s %s", name, found[name].Outcome, found[name].Got, found[name].Detail)
		}
	}
	record := filepath.Join(subject.reports, graded.RunID, healthRecord)
	kept, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("the /healthz answer was not kept: %v", err)
	}
	if string(kept) != unrecordedReadHealth {
		t.Errorf("kept %q, want the answer as read", kept)
	}
	if found["health/reads_unrecorded"].Source != record {
		t.Errorf("health/reads_unrecorded cites %q, want %q", found["health/reads_unrecorded"].Source, record)
	}
}

func TestAHealthCountTheAnswerDoesNotMatchFails(t *testing.T) {
	subject, scenario := healthLab(t, "{ reads_unrecorded: 0 }", unrecordedReadHealth)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["health/reads_unrecorded"]; result.Outcome != assertion.Fail || result.Got != "1" {
		t.Errorf("health/reads_unrecorded is %s, got %q", result.Outcome, result.Got)
	}
}

func TestAHealthNobodyCouldReadFailsEveryStatedCount(t *testing.T) {
	subject, scenario := healthLab(t, "{ reads_unrecorded: 0 }", "")
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["health/reads_unrecorded"]; result.Outcome != assertion.Fail {
		t.Errorf("health/reads_unrecorded is %s: %s", result.Outcome, result.Detail)
	}
	if _, err := os.Stat(filepath.Join(subject.reports, graded.RunID, healthRecord)); !os.IsNotExist(err) {
		t.Errorf("a record was kept from no answer: %v", err)
	}
}

func TestNoHealthCheckRunsWhereNoneIsStated(t *testing.T) {
	subject, scenario := healthLab(t, "", unrecordedReadHealth)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for name := range results(graded) {
		if strings.HasPrefix(name, "health") {
			t.Errorf("%s ran on a scenario that states no health", name)
		}
	}
	if graded.Outcome() != assertion.Pass {
		t.Errorf("the run is %s", graded.Outcome())
	}
}

// The drain polls until the spool is empty; the record kept is the answer that
// ended the wait, not an earlier one taken while records were still in flight.
func TestTheKeptHealthIsTheLastAnswer(t *testing.T) {
	subject, scenario := healthLab(t, "{ reads_unrecorded: 1 }", unrecordedReadHealth)
	inFlight := strings.Replace(unrecordedReadHealth, `"unacknowledged":0`, `"unacknowledged":64`, 1)
	inFlight = strings.Replace(inFlight, `"reads_unrecorded":1`, `"reads_unrecorded":0`, 1)
	polls := 0
	answer := subject.compose.(*fakeCompose).exec
	subject.compose.(*fakeCompose).exec = func(service string, args []string) Split {
		if strings.HasSuffix(args[len(args)-1], "/healthz") {
			if polls++; polls == 1 {
				return Split{Stdout: "status 200\n" + inFlight}
			}
		}
		return answer(service, args)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if polls < 2 {
		t.Fatalf("the drain polled /healthz %d time(s); the test needs a second answer", polls)
	}
	kept, err := os.ReadFile(filepath.Join(subject.reports, graded.RunID, healthRecord))
	if err != nil || string(kept) != unrecordedReadHealth {
		t.Errorf("kept %q (%v), want the answer that ended the wait", kept, err)
	}
	if found := results(graded)["health/reads_unrecorded"]; found.Outcome != assertion.Pass {
		t.Errorf("health/reads_unrecorded is %s: %s", found.Outcome, found.Got)
	}
}
