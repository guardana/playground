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

// A scan of a pickle as the verifier prints it, cut to what a scan step reads.
const scanReport = `{"schema_version": 18,
 "run": {"target": {"type": "artifact", "ref": "target"},
  "result_summary": {"rules_run": ["guardana.supply_chain.pickle_opcode"], "gate": "fail"},
  "coverage": {"shortfall": []}},
 "scope": {"files": ["target/config.json", "target/model.pkl"], "excludes": []},
 "findings": [{"rule_id": "guardana.supply_chain.pickle_opcode", "severity": "CRITICAL", "target_ref": "target/model.pkl",
   "evidence": {"summary": "unpickling imports 1 non-allowlisted callable(s): clab_never_a_module.nothing"}}],
 "unverified": [], "waived": [], "errors": []}`

func scanExpectation() labspec.VerifierExpectation {
	return labspec.VerifierExpectation{
		ExitCode: exitCode(1), Gate: "fail", Shortfall: shortfalls(), Errors: sources(),
		Scope: sources("model.pkl", "config.json"),
		Findings: &[]labspec.FindingExpectation{{RuleID: "guardana.supply_chain.pickle_opcode", Severity: "CRITICAL",
			File: "model.pkl", SummaryContains: "clab_never_a_module"}},
	}
}

func gradeScan(t *testing.T, want labspec.VerifierExpectation, report string) map[string]assertion.Result {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "step-1.json"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := labspec.Scenario{
		Verifier: []labspec.VerifierStep{{Scan: &labspec.ScanStep{Target: "artifacts/pickle-import/defect"}}},
		Expect:   labspec.Expect{Verifier: map[int]labspec.VerifierExpectation{1: want}},
	}
	runs := []check.VerifierRun{{Step: 1, URL: "target", Ran: true, ExitCode: 1, Report: filepath.Join(dir, "step-1.json")}}
	results, err := check.Verifier{Scenario: spec, Runs: runs}.Run(context.Background(), assertion.Records{})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]assertion.Result{}
	for _, result := range results {
		byName[result.Check] = result
	}
	return byName
}

func TestAScanIsGradedOnTheExactFilesAndFindings(t *testing.T) {
	got := gradeScan(t, scanExpectation(), scanReport)
	for _, name := range []string{"report", "exit-code", "gate", "shortfall", "errors", "scope", "findings", "unverified", "waived"} {
		if result := got["verifier/step-1/"+name]; result.Outcome != assertion.Pass {
			t.Errorf("step-1/%s = %s (%q)", name, result.Outcome, result.Got)
		}
	}
}

func TestEachPartOfAScansRecordFailsOnItsOwn(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*labspec.VerifierExpectation)
		report func(string) string
		fails  string
	}{
		"a file the scan did not list":  {func(w *labspec.VerifierExpectation) { *w.Scope = append(*w.Scope, "train.py") }, nil, "scope"},
		"a file the scan listed beside": {func(w *labspec.VerifierExpectation) { *w.Scope = []string{"model.pkl"} }, nil, "scope"},
		"no scope in the report": {nil, func(r string) string {
			return strings.Replace(r, `"scope": {"files": ["target/config.json", "target/model.pkl"], "excludes": []}`, `"scope": null`, 1)
		}, "scope"},
		"the finding about another file": {func(w *labspec.VerifierExpectation) { (*w.Findings)[0].File = "config.json" }, nil, "findings"},
		"another severity":               {func(w *labspec.VerifierExpectation) { (*w.Findings)[0].Severity = "HIGH" }, nil, "findings"},
		"no finding wanted":              {func(w *labspec.VerifierExpectation) { *w.Findings = nil }, nil, "findings"},
		"an unverified result unnamed": {nil, func(r string) string {
			return strings.Replace(r, `"unverified": []`, `"unverified": [{"rule_id": "guardana.supply_chain.provenance", "severity": "INFO", "target_ref": "target"}]`, 1)
		}, "unverified"},
		"a waived finding": {nil, func(r string) string {
			return strings.Replace(r, `"waived": []`, `"waived": [{"rule_id": "guardana.supply_chain.provenance", "severity": "MEDIUM", "target_ref": "target/config.json"}]`, 1)
		}, "waived"},
	} {
		t.Run(name, func(t *testing.T) {
			want, report := scanExpectation(), scanReport
			if test.change != nil {
				test.change(&want)
			}
			if test.report != nil {
				report = test.report(report)
			}
			if result := gradeScan(t, want, report)["verifier/step-1/"+test.fails]; result.Outcome != assertion.Fail {
				t.Errorf("step-1/%s = %s (%q), want fail", test.fails, result.Outcome, result.Got)
			}
		})
	}
}

// A finding the scenario does not name is one nobody expected: the list is
// exact in both directions.
func TestAnUnnamedFindingFailsTheScan(t *testing.T) {
	report := strings.Replace(scanReport, `"unverified": []`, `"unverified": []`, 1)
	report = strings.Replace(report, `"findings": [{`, `"findings": [{"rule_id": "guardana.supply_chain.provenance", "severity": "MEDIUM", "target_ref": "target/config.json", "evidence": {"summary": "no provenance"}}, {`, 1)
	if result := gradeScan(t, scanExpectation(), report)["verifier/step-1/findings"]; result.Outcome != assertion.Fail {
		t.Errorf("step-1/findings = %s (%q), want fail", result.Outcome, result.Got)
	}
}

// A finding on a line is named file:line; the file is what a scenario states.
func TestAFindingOnALineIsAboutItsFile(t *testing.T) {
	report := strings.Replace(scanReport, `"target_ref": "target/model.pkl"`, `"target_ref": "target/model.pkl:5"`, 1)
	if result := gradeScan(t, scanExpectation(), report)["verifier/step-1/findings"]; result.Outcome != assertion.Pass {
		t.Errorf("step-1/findings = %s (%q), want pass", result.Outcome, result.Got)
	}
	report = strings.Replace(scanReport, `"target_ref": "target/model.pkl"`, `"target_ref": "target/model.pkl.bak:5"`, 1)
	if result := gradeScan(t, scanExpectation(), report)["verifier/step-1/findings"]; result.Outcome != assertion.Fail {
		t.Errorf("a finding on another file: step-1/findings = %s (%q), want fail", result.Outcome, result.Got)
	}
}
