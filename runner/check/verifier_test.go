package check_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

const (
	fsURL = "http://victim-fs:8080/mcp"
	// A drift report as the verifier prints it, cut to the fields a scenario
	// reads and a few it does not.
	driftReport = `{"schema_version": 6, "$schema": "https://guardana.dev/schemas/run/v6.schema.json",
 "run": {"target": {"type": "endpoint", "ref": "http://victim-fs:8080/mcp"},
  "result_summary": {"findings": 2, "unverified": 1, "rules_run": ["guardana.agent.mcp_server_manifest",
   "guardana.mcp.unauthenticated_access", "guardana.mcp.session_binding", "guardana.mcp.cache_scope",
   "guardana.mcp.scope_breadth"]}},
 "findings": [
  {"rule_id": "guardana.agent.mcp_server_manifest", "severity": "CRITICAL", "verdict": null,
   "evidence": {"summary": "the declaration of 'fs.read' changed after it was approved (rug pull)"}},
  {"rule_id": "guardana.mcp.unauthenticated_access", "severity": "LOW",
   "evidence": {"summary": "the server returns its tool manifest to a caller presenting no credential"}}],
 "unverified": [{"rule_id": "guardana.mcp.session_binding", "severity": "HIGH",
   "evidence": {"summary": "not settled"}, "verdict": {"outcome": "inconclusive"}}],
 "errors": [{"source": "guardana.mcp.scope_breadth", "stage": "run", "reason": "RuntimeError: x"}],
 "waived": [], "observations": [], "assessments": []}`
	fsPin = `{"schema_version": 2, "server": "http://victim-fs:8080/mcp", "tools": {"fs.read": "sha256:00"}}`
)

func exitCode(code int) *int { return &code }

func verifierSpec(want labspec.VerifierExpectation) labspec.Scenario {
	return labspec.Scenario{
		Verifier: []labspec.VerifierStep{
			{Probe: &labspec.ProbeStep{Server: "victim-fs", WritePin: true}},
			{Probe: &labspec.ProbeStep{Server: "victim-fs", PinFrom: 1}},
		},
		Expect: labspec.Expect{Verifier: map[int]labspec.VerifierExpectation{
			1: {ExitCode: exitCode(0)},
			2: want,
		}},
	}
}

func driftExpectation() labspec.VerifierExpectation {
	return labspec.VerifierExpectation{
		ExitCode: exitCode(1),
		FindingsInclude: []labspec.FindingExpectation{
			{RuleID: "guardana.agent.mcp_server_manifest", SummaryContains: "'fs.read'"},
			{RuleID: "guardana.mcp.unauthenticated_access", Severity: "LOW"},
		},
		FindingsExclude:   []string{"guardana.mcp.cache_scope"},
		UnverifiedInclude: []string{"guardana.mcp.session_binding"},
	}
}

// writeRuns writes the pin and the report into a run directory and returns
// the two steps as the runner would record them.
func writeRuns(t *testing.T, pin, report string) []check.VerifierRun {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"pin-1.json": pin, "step-2.json": report}
	for name, body := range files {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return []check.VerifierRun{
		{Step: 1, Probe: labspec.ProbeStep{Server: "victim-fs", WritePin: true}, URL: fsURL, Ran: true,
			Report: filepath.Join(dir, "step-1.json"), Pin: filepath.Join(dir, "pin-1.json")},
		{Step: 2, Probe: labspec.ProbeStep{Server: "victim-fs", PinFrom: 1}, URL: fsURL, Ran: true, ExitCode: 1,
			Report: filepath.Join(dir, "step-2.json")},
	}
}

func gradeVerifier(t *testing.T, spec labspec.Scenario, runs []check.VerifierRun) map[string]assertion.Result {
	t.Helper()
	results, err := check.Verifier{Scenario: spec, Runs: runs}.Run(context.Background(), assertion.Records{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]assertion.Result{}
	for _, result := range results {
		if _, twice := byName[result.Check]; twice {
			t.Errorf("%s reported twice", result.Check)
		}
		byName[result.Check] = result
	}
	return byName
}

func wantOutcomes(t *testing.T, got map[string]assertion.Result, want map[string]assertion.Outcome) {
	t.Helper()
	for name, outcome := range want {
		result, found := got[name]
		if !found {
			t.Errorf("no %s result among %d", name, len(got))
			continue
		}
		if result.Outcome != outcome {
			t.Errorf("%s = %s, want %s (got %q: %s)", name, result.Outcome, outcome, result.Got, result.Detail)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d results, want %d: %v", len(got), len(want), got)
	}
}

func TestADriftReportIsGradedFromItsFindings(t *testing.T) {
	got := gradeVerifier(t, verifierSpec(driftExpectation()), writeRuns(t, fsPin, driftReport))
	pass := assertion.Pass
	wantOutcomes(t, got, map[string]assertion.Outcome{
		"verifier/step-1/pin":                                         pass,
		"verifier/step-1/exit-code":                                   pass,
		"verifier/step-2/report":                                      pass,
		"verifier/step-2/exit-code":                                   pass,
		"verifier/step-2/finding/guardana.agent.mcp_server_manifest":  pass,
		"verifier/step-2/finding/guardana.mcp.unauthenticated_access": pass,
		"verifier/step-2/no-finding/guardana.mcp.cache_scope":         pass,
		"verifier/step-2/unverified/guardana.mcp.session_binding":     pass,
	})
}

func TestEachStatedExpectationCanFailOnItsOwn(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*labspec.VerifierExpectation)
		red    string
	}{
		"another exit code": {func(w *labspec.VerifierExpectation) { w.ExitCode = exitCode(0) }, "verifier/step-2/exit-code"},
		"another severity": {func(w *labspec.VerifierExpectation) { w.FindingsInclude[1].Severity = "HIGH" },
			"verifier/step-2/finding/guardana.mcp.unauthenticated_access"},
		"another summary": {func(w *labspec.VerifierExpectation) { w.FindingsInclude[0].SummaryContains = "'fs.write'" },
			"verifier/step-2/finding/guardana.agent.mcp_server_manifest"},
		"a finding never reported": {func(w *labspec.VerifierExpectation) { w.FindingsInclude[1].RuleID = "guardana.mcp.token_audience" },
			"verifier/step-2/finding/guardana.mcp.token_audience"},
		"a rule excluded that reported": {func(w *labspec.VerifierExpectation) { w.FindingsExclude[0] = "guardana.agent.mcp_server_manifest" },
			"verifier/step-2/no-finding/guardana.agent.mcp_server_manifest"},
		"a rule excluded that could not tell": {func(w *labspec.VerifierExpectation) {
			w.FindingsExclude[0], w.UnverifiedInclude = "guardana.mcp.session_binding", nil
		}, "verifier/step-2/no-finding/guardana.mcp.session_binding"},
		"a rule excluded that never ran": {func(w *labspec.VerifierExpectation) { w.FindingsExclude[0] = "guardana.mcp.issuer_identification" },
			"verifier/step-2/no-finding/guardana.mcp.issuer_identification"},
		"a rule excluded that raised": {func(w *labspec.VerifierExpectation) { w.FindingsExclude[0] = "guardana.mcp.scope_breadth" },
			"verifier/step-2/no-finding/guardana.mcp.scope_breadth"},
		"an unverified rule that concluded": {func(w *labspec.VerifierExpectation) { w.UnverifiedInclude[0] = "guardana.mcp.cache_scope" },
			"verifier/step-2/unverified/guardana.mcp.cache_scope"},
	} {
		t.Run(name, func(t *testing.T) {
			want := driftExpectation()
			test.change(&want)
			got := gradeVerifier(t, verifierSpec(want), writeRuns(t, fsPin, driftReport))
			for check, result := range got {
				wanted := assertion.Pass
				if check == test.red {
					wanted = assertion.Fail
				}
				if result.Outcome != wanted {
					t.Errorf("%s = %s, want %s: %s", check, result.Outcome, wanted, result.Detail)
				}
			}
			if _, found := got[test.red]; !found {
				t.Errorf("no %s result", test.red)
			}
		})
	}
}

// A report that is missing or about something else is a failed record, and
// nothing read beside it is graded: the exit status included, because a
// container that never reached the verifier exits non-zero as well.
func TestAMissingOrForeignRecordGradesNothingBesideIt(t *testing.T) {
	foreign := strings.Replace(driftReport, `"ref": "http://victim-fs:8080/mcp"`, `"ref": "http://victim-crm:8080/mcp"`, 1)
	for name, test := range map[string]struct{ pin, report string }{
		"no report":             {fsPin, ""},
		"a report of another":   {fsPin, foreign},
		"a report not JSON":     {fsPin, "Error: no such option: --mcp-pin"},
		"another report schema": {fsPin, strings.Replace(driftReport, `"schema_version": 6`, `"schema_version": 7`, 1)},
		"no rule ran":           {fsPin, strings.Replace(driftReport, `"rules_run": [`, `"rules_run": [], "x": [`, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			got := gradeVerifier(t, verifierSpec(driftExpectation()), writeRuns(t, test.pin, test.report))
			if got["verifier/step-2/report"].Outcome != assertion.Fail {
				t.Errorf("report = %s, want fail", got["verifier/step-2/report"].Outcome)
			}
			for name, result := range got {
				if strings.HasPrefix(name, "verifier/step-2/") && name != "verifier/step-2/report" &&
					result.Outcome != assertion.Indeterminate {
					t.Errorf("%s = %s beside an unread report, want indeterminate", name, result.Outcome)
				}
			}
		})
	}
}

func TestAPinStepIsGradedFromThePinItWrote(t *testing.T) {
	for name, pin := range map[string]string{
		"no pin":               "",
		"a pin of another":     strings.Replace(fsPin, "victim-fs", "victim-crm", 1),
		"a pin of no tool":     strings.Replace(fsPin, `{"fs.read": "sha256:00"}`, `{}`, 1),
		"another pin schema":   strings.Replace(fsPin, `"schema_version": 2`, `"schema_version": 1`, 1),
		"a pin that is prose ": "Wrote 3 approved tool description(s)",
	} {
		t.Run(name, func(t *testing.T) {
			got := gradeVerifier(t, verifierSpec(driftExpectation()), writeRuns(t, pin, driftReport))
			if got["verifier/step-1/pin"].Outcome != assertion.Fail {
				t.Errorf("pin = %s, want fail", got["verifier/step-1/pin"].Outcome)
			}
			if got["verifier/step-1/exit-code"].Outcome != assertion.Indeterminate {
				t.Errorf("exit-code = %s beside no pin, want indeterminate", got["verifier/step-1/exit-code"].Outcome)
			}
		})
	}
}

func TestAStepThatNeverRanEstablishesNothing(t *testing.T) {
	runs := writeRuns(t, fsPin, driftReport)
	runs[1].Ran, runs[1].Detail = false, "docker compose run: no such service"
	got := gradeVerifier(t, verifierSpec(driftExpectation()), runs[1:])
	wantOutcomes(t, got, map[string]assertion.Outcome{
		"verifier/step-1/ran": assertion.Indeterminate,
		"verifier/step-2/ran": assertion.Indeterminate,
	})
}

// A waived finding is a finding someone accepted, not a rule that found
// nothing. The entry has the shape the verifier's report writer gives every
// channel of findings.
func TestAWaivedFindingIsNotAnExcludedOne(t *testing.T) {
	waived := strings.Replace(driftReport, `"waived": []`, `"waived": [{"rule_id": "guardana.mcp.cache_scope",
  "severity": "MEDIUM", "title": "x", "taxonomy": [], "target_ref": "http://victim-fs:8080/mcp",
  "evidence": {"summary": "a response is cached across callers", "detail": ""}, "verdict": null}]`, 1)
	if waived == driftReport {
		t.Fatal("the replacement changed nothing")
	}
	got := gradeVerifier(t, verifierSpec(driftExpectation()), writeRuns(t, fsPin, waived))
	result := got["verifier/step-2/no-finding/guardana.mcp.cache_scope"]
	if result.Outcome != assertion.Fail || !strings.Contains(result.Got, "waived") {
		t.Errorf("no-finding = %s (%q), want a failure naming the waiver", result.Outcome, result.Got)
	}
}
