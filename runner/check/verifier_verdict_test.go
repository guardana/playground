package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// An undecided run as the verifier prints it, cut to the fields a scenario
// reads: the gate it recorded, the coverage it lacked and a rule that raised.
const undecidedReport = `{"schema_version": 18,
 "run": {"target": {"ref": "http://victim-fs:8080/mcp"},
  "result_summary": {"rules_run": ["guardana.mcp.unauthenticated_access"], "gate": "indeterminate"},
  "coverage": {"shortfall": [
   {"kind": "demanded_check", "name": "guardana.agent.mcp_server_manifest", "detail": "approved for another server"},
   {"kind": "demanded_check", "name": "guardana.mcp.session_binding", "detail": "not finished"}]}},
 "findings": [], "unverified": [], "waived": [],
 "errors": [{"source": "guardana.mcp.scope_breadth", "stage": "run", "reason": "RuntimeError: x"}]}`

func shortfalls(gaps ...labspec.ShortfallExpectation) *[]labspec.ShortfallExpectation { return &gaps }

func sources(names ...string) *[]string { return &names }

func undecidedExpectation() labspec.VerifierExpectation {
	return labspec.VerifierExpectation{
		ExitCode: exitCode(2),
		Gate:     "indeterminate",
		// The nameless entry comes first: it may stand for either gap, and
		// only the named one decides which.
		Shortfall: shortfalls(
			labspec.ShortfallExpectation{Kind: "demanded_check"},
			labspec.ShortfallExpectation{Kind: "demanded_check", Name: "guardana.agent.mcp_server_manifest"},
		),
		Errors: sources("guardana.mcp.scope_breadth"),
	}
}

func undecidedRuns(t *testing.T, report string) []check.VerifierRun {
	t.Helper()
	runs := writeRuns(t, fsPin, report)
	runs[1].ExitCode = 2
	return runs
}

func TestTheVerdictAndItsCauseAreGradedExactly(t *testing.T) {
	got := gradeVerifier(t, verifierSpec(undecidedExpectation()), undecidedRuns(t, undecidedReport))
	pass := assertion.Pass
	wantOutcomes(t, got, map[string]assertion.Outcome{
		"verifier/step-1/pin":       pass,
		"verifier/step-1/exit-code": pass,
		"verifier/step-2/report":    pass,
		"verifier/step-2/exit-code": pass,
		"verifier/step-2/gate":      pass,
		"verifier/step-2/shortfall": pass,
		"verifier/step-2/errors":    pass,
	})
}

func TestEachVerdictExpectationFailsOnItsOwn(t *testing.T) {
	for name, test := range map[string]struct {
		change func(*labspec.VerifierExpectation)
		fails  string
	}{
		"another gate": {func(w *labspec.VerifierExpectation) { w.Gate = "fail" }, "gate"},
		"one gap fewer": {func(w *labspec.VerifierExpectation) {
			w.Shortfall = shortfalls(labspec.ShortfallExpectation{Kind: "demanded_check"})
		}, "shortfall"},
		"no gap": {func(w *labspec.VerifierExpectation) { w.Shortfall = shortfalls() }, "shortfall"},
		"a gap about another rule": {func(w *labspec.VerifierExpectation) {
			(*w.Shortfall)[1].Name = "guardana.mcp.cache_scope"
		}, "shortfall"},
		"a gap of another kind": {func(w *labspec.VerifierExpectation) {
			(*w.Shortfall)[0].Kind = "empty_target"
		}, "shortfall"},
		"no error":            {func(w *labspec.VerifierExpectation) { w.Errors = sources() }, "errors"},
		"an error of another": {func(w *labspec.VerifierExpectation) { w.Errors = sources("guardana.mcp.cache_scope") }, "errors"},
	} {
		t.Run(name, func(t *testing.T) {
			want := undecidedExpectation()
			test.change(&want)
			got := gradeVerifier(t, verifierSpec(want), undecidedRuns(t, undecidedReport))
			for _, what := range []string{"gate", "shortfall", "errors"} {
				outcome, wanted := got["verifier/step-2/"+what].Outcome, assertion.Pass
				if what == test.fails {
					wanted = assertion.Fail
				}
				if outcome != wanted {
					t.Errorf("step-2/%s = %s, want %s", what, outcome, wanted)
				}
			}
		})
	}
}

// A report silent about what a step asserts has not said "none": the
// expectation fails rather than reading an absent list as an empty one.
func TestAReportSilentOnTheVerdictFailsWhatTheStepAsserts(t *testing.T) {
	for name, test := range map[string]struct{ from, to, fails string }{
		"no gate":          {`, "gate": "indeterminate"`, ``, "gate"},
		"a null gate":      {`"gate": "indeterminate"`, `"gate": null`, "gate"},
		"no coverage":      {`"coverage": {"shortfall": [`, `"x": {"y": [`, "shortfall"},
		"a null shortfall": {`"coverage": {"shortfall": [`, `"coverage": {"shortfall": null, "x": [`, "shortfall"},
		"no errors":        {`"errors": [`, `"x": [`, "errors"},
		"errors that are null": {`"errors": [{"source": "guardana.mcp.scope_breadth", "stage": "run", "reason": "RuntimeError: x"}]`,
			`"errors": null`, "errors"},
	} {
		t.Run(name, func(t *testing.T) {
			report := strings.Replace(undecidedReport, test.from, test.to, 1)
			if report == undecidedReport {
				t.Fatal("the replacement changed nothing")
			}
			got := gradeVerifier(t, verifierSpec(undecidedExpectation()), undecidedRuns(t, report))
			if result := got["verifier/step-2/"+test.fails]; result.Outcome != assertion.Fail {
				t.Errorf("step-2/%s = %s (%q), want fail", test.fails, result.Outcome, result.Got)
			}
		})
	}
}

// Two gaps alike in kind and name, told apart only by their detail, are two
// gaps: each expectation takes one.
func TestTwoAlikeGapsAreMatchedOneEach(t *testing.T) {
	report := strings.Replace(undecidedReport, `"name": "guardana.mcp.session_binding"`,
		`"name": "guardana.agent.mcp_server_manifest"`, 1)
	if report == undecidedReport {
		t.Fatal("the replacement changed nothing")
	}
	manifest := labspec.ShortfallExpectation{Kind: "demanded_check", Name: "guardana.agent.mcp_server_manifest"}
	want := undecidedExpectation()
	want.Shortfall = shortfalls(manifest, manifest)
	got := gradeVerifier(t, verifierSpec(want), undecidedRuns(t, report))
	if result := got["verifier/step-2/shortfall"]; result.Outcome != assertion.Pass {
		t.Errorf("step-2/shortfall = %s (%q), want pass", result.Outcome, result.Got)
	}
}

func TestATraceIsGradedOnItsVerdictToo(t *testing.T) {
	runs := undecidedRuns(t, undecidedReport)
	want := undecidedExpectation()
	want.Gate = "pass"
	results, err := check.Trace{Analysis: runs[1], Want: want}.Run(context.Background(), assertion.Records{})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := map[string]assertion.Outcome{}
	for _, result := range results {
		outcomes[result.Check] = result.Outcome
	}
	if outcomes["trace/gate"] != assertion.Fail || outcomes["trace/shortfall"] != assertion.Pass {
		t.Errorf("trace results = %v, want trace/gate fail and trace/shortfall pass", outcomes)
	}
}
