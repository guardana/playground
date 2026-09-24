package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const contractFinding = "contract.lab-bank-account.bank-account-needs-approval"

func thisRunsTrace(runID string) string { return "/lab-run/trace.jsonl#" + runID }

// traceLab is an enforcer lab whose scenario has the verifier grade the
// agent's trace against a contract; the fake verifier answers with a report
// about target, carrying the finding when found is set.
func traceLab(t *testing.T, target func(runID string) string, found bool) (lab, *fakeCompose, string) {
	t.Helper()
	subject, compose, scenario := enforcerLab(t)
	body, err := os.ReadFile(scenario)
	if err != nil {
		t.Fatal(err)
	}
	withTrace := strings.Replace(strings.Replace(string(body), "profile: [core, enforcer]", "profile: [core, enforcer, trace]", 1), "expect:\n",
		"trace: { contract: config/contracts/lab.yaml, ai_system: support-agent }\nexpect:\n  trace: { exit_code: 1, findings_include: [ { rule_id: "+contractFinding+", severity: HIGH } ] }\n", 1)
	writeFile(scenario, withTrace)
	writeFile(filepath.Join(subject.root, "config/contracts/lab.yaml"), "rules: []\n")
	compose.agentTrace = `{"guardana_trace":3}` + "\n"
	compose.inProfile = append(compose.inProfile, traceService)
	compose.split = func(service, _ string, args []string) (Split, error) {
		if service != "trace-verifier" || args[0] != "analyze-trace" || !slices.Contains(args, "/contracts/lab.yaml") {
			return Split{}, fmt.Errorf("unexpected verifier call %v", args)
		}
		if _, err := os.Stat(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "verifier", "trace.jsonl")); err != nil {
			return Split{}, fmt.Errorf("the trace was not handed to the verifier: %w", err)
		}
		findings := "[]"
		if found {
			findings = `[{"rule_id":"` + contractFinding + `","severity":"HIGH"}]`
		}
		report := `{"schema_version":6,"run":{"target":{"ref":"` + target(compose.env["LAB_RUN_ID"]) +
			`"},"result_summary":{"rules_run":["` + contractFinding + `"]}},"findings":` + findings +
			`,"unverified":[],"waived":[],"errors":[]}`
		return Split{ExitCode: 1, Stdout: report}, nil
	}
	pinVerifierImage(t, &subject, "0.26.1")
	return subject, compose, scenario
}

func traceResults(t *testing.T, subject lab, scenario string) map[string]assertion.Result {
	t.Helper()
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := map[string]assertion.Result{}
	for _, result := range graded.Results {
		if strings.HasPrefix(result.Check, "trace/") {
			found[result.Check] = result
		}
	}
	if len(found) == 0 {
		t.Fatal("the run was not graded on the verifier's analysis of its trace")
	}
	return found
}

func TestTheVerifierGradesTheAgentsTraceAgainstTheContract(t *testing.T) {
	subject, compose, scenario := traceLab(t, thisRunsTrace, true)
	for name, result := range traceResults(t, subject, scenario) {
		if result.Outcome != assertion.Pass {
			t.Errorf("%s is %s: %s %s", name, result.Outcome, result.Got, result.Detail)
		}
	}
	replay := strings.Join(compose.ran[len(compose.ran)-2], " ")
	if !strings.Contains(replay, "-trace /reports/") {
		t.Errorf("the agent was not asked for a trace: %s", replay)
	}
	if slices.Contains(compose.broughtUp, traceService) {
		t.Error("the one-shot trace verifier was brought up with the victims")
	}
}

func TestAReportAboutAnotherTraceIsNotThisRunsVerdict(t *testing.T) {
	subject, _, scenario := traceLab(t, func(string) string { return "/lab-run/trace.jsonl#another-run" }, true)
	results := traceResults(t, subject, scenario)
	if results["trace/report"].Outcome != assertion.Fail {
		t.Errorf("a report about another run's trace was %s", results["trace/report"].Outcome)
	}
}

func TestAContractThatHeldIsNotTheFindingTheScenarioWants(t *testing.T) {
	subject, _, scenario := traceLab(t, thisRunsTrace, false)
	failed := 0
	for _, result := range traceResults(t, subject, scenario) {
		if result.Outcome == assertion.Fail {
			failed++
		}
	}
	if failed == 0 {
		t.Error("a report without the wanted finding passed")
	}
}

func TestTheTraceIsTakenOnlyAsARegularFileWithinItsBound(t *testing.T) {
	dir := t.TempDir()
	outside, _ := outsideFile(t)
	within := filepath.Join(dir, "within.jsonl")
	writeFile(within, strings.Repeat("x", maxTraceBytes))
	past := filepath.Join(dir, "past.jsonl")
	writeFile(past, strings.Repeat("x", maxTraceBytes+1))
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := copyTrace(within, filepath.Join(dir, "to-within")); err != nil {
		t.Errorf("a trace at the bound was refused: %v", err)
	}
	for name, from := range map[string]string{"past the bound": past, "a link": link, "a directory": dir} {
		if err := copyTrace(from, filepath.Join(dir, "to-"+strings.ReplaceAll(name, " ", "-"))); err == nil {
			t.Errorf("a trace that is %s was taken", name)
		}
	}
	for name, planted := range map[string]bool{"an empty directory": false, "a directory holding a link": true} {
		existing := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		if err := os.Mkdir(existing, 0o700); err != nil {
			t.Fatal(err)
		}
		if planted {
			if err := os.Symlink(outside, filepath.Join(existing, traceFile)); err != nil {
				t.Fatal(err)
			}
		}
		if err := copyTrace(within, existing); err == nil {
			t.Errorf("the trace was handed over into %s the runner did not make", name)
		}
	}
}
