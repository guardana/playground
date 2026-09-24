package report_test

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/report"
)

func mixed() assertion.Report {
	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	return assertion.Report{
		Scenario:  "flow-02-private-to-public-sink",
		RunID:     "flow-02-20260909T120000Z-a1b2c3d4",
		StartedAt: started,
		EndedAt:   started.Add(1500 * time.Millisecond),
		Results: []assertion.Result{
			{Check: "boot/victim-fs", Outcome: assertion.Pass, Want: "running", Got: "running", Source: "reports/r/boot.json"},
			{
				Check: "decisions/step-3", Outcome: assertion.Fail,
				Want: "DENY", Got: "ALLOW", Source: "reports/r/evidence.jsonl:6",
				Detail: "the recorded verdict is ALLOW and the scenario expects DENY",
			},
			{
				Check: "network-isolation/victim-unreachable", Outcome: assertion.Indeterminate,
				Want: "no route to victim-fs:8080", Got: "the probe did not run",
				Source: "reports/r/boot.json", Detail: "the agent image could not be run",
			},
		},
	}
}

// JUnit has a <skipped> element and every tool that reads JUnit treats it as
// "not a failure". An indeterminate result is a run that established nothing,
// and writing it as skipped would hand that to CI as green.
type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitDocument struct {
	Tests    int `xml:"tests,attr"`
	Failures int `xml:"failures,attr"`
	Suites   []struct {
		Cases []struct {
			Name    string        `xml:"name,attr"`
			Failure *junitFailure `xml:"failure"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

func parseJUnit(t *testing.T, document string) junitDocument {
	t.Helper()
	var parsed junitDocument
	if err := xml.Unmarshal([]byte(document), &parsed); err != nil {
		t.Fatalf("the document is not XML: %v\n%s", err, document)
	}
	return parsed
}

func TestJUnitWritesIndeterminateAsAFailureAndNeverAsSkipped(t *testing.T) {
	var out strings.Builder
	if err := report.WriteJUnit(&out, mixed()); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	document := out.String()
	if strings.Contains(strings.ToLower(document), "skip") {
		t.Errorf("the document mentions skipping:\n%s", document)
	}

	parsed := parseJUnit(t, document)
	if len(parsed.Suites) != 1 || len(parsed.Suites[0].Cases) != 3 {
		t.Fatalf("want one suite of three cases, got %+v", parsed.Suites)
	}
	cases := parsed.Suites[0].Cases
	if cases[0].Failure != nil {
		t.Errorf("a passing result carries a failure element: %+v", cases[0])
	}
	if failure := cases[1].Failure; failure == nil || failure.Type != "fail" {
		t.Errorf("the failing result is not written as a failure: %+v", cases[1])
	}
	indeterminate := cases[2].Failure
	if indeterminate == nil {
		t.Fatalf("the indeterminate result carries no failure element: %+v", cases[2])
	}
	if indeterminate.Type != "indeterminate" {
		t.Errorf("failure type is %q, want %q", indeterminate.Type, "indeterminate")
	}
	if !strings.Contains(indeterminate.Message, "nothing was established") {
		t.Errorf("failure message does not say nothing was established: %q", indeterminate.Message)
	}
	if !strings.Contains(indeterminate.Body, "the agent image could not be run") {
		t.Errorf("failure body drops the detail: %q", indeterminate.Body)
	}
}

func TestJUnitCountsEveryResultAsATestcase(t *testing.T) {
	var out strings.Builder
	if err := report.WriteJUnit(&out, mixed()); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	parsed := parseJUnit(t, out.String())
	if parsed.Tests != 3 || parsed.Failures != 2 {
		t.Errorf("tests=%d failures=%d, want 3 and 2", parsed.Tests, parsed.Failures)
	}
	if len(parsed.Suites) != 1 || len(parsed.Suites[0].Cases) != 3 {
		t.Fatalf("want one suite of three cases, got %+v", parsed.Suites)
	}
}

// Zero testcases is the other shape a proved-nothing run can take, and every
// JUnit reader calls an empty suite green.
func TestJUnitRefusesToWriteAnEmptySuiteAsGreen(t *testing.T) {
	empty := assertion.Report{Scenario: "flow-02", RunID: "run-1"}

	var out strings.Builder
	if err := report.WriteJUnit(&out, empty); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	document := out.String()

	parsed := parseJUnit(t, document)
	if parsed.Tests == 0 || parsed.Failures == 0 {
		t.Errorf("a report with no results wrote tests=%d failures=%d:\n%s",
			parsed.Tests, parsed.Failures, document)
	}
}

// A detail is whatever a service, a probe or a build log said, and nothing
// sanitises it on the way in. The assertions are on the elements a consumer
// parses: failures= is a number this writer computes from the outcomes, so it
// reads the same whether or not the text broke out of the element it was
// written into, and a forged <failure message="green"/> passes it.
func TestJUnitEscapesWhatAServiceSaid(t *testing.T) {
	payloads := map[string]string{
		"a forged failure element": `a service said <failure message="green"/> & meant nothing by it`,
		"an attempt to close the case and open another": `nothing</failure></testcase>` +
			`<testcase name="forged" classname="flow-02" time="0"/><testcase>`,
	}

	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			hostile := mixed()
			hostile.Results[1].Detail = payload

			var out strings.Builder
			if err := report.WriteJUnit(&out, hostile); err != nil {
				t.Fatalf("WriteJUnit: %v", err)
			}
			document := out.String()
			parsed := parseJUnit(t, document)
			if len(parsed.Suites) != 1 {
				t.Fatalf("want one suite, got %d:\n%s", len(parsed.Suites), document)
			}
			cases := parsed.Suites[0].Cases
			if len(cases) != 3 {
				t.Fatalf("the document holds %d testcase elements, want the 3 results:\n%s",
					len(cases), document)
			}
			carrying := 0
			for _, one := range cases {
				if one.Failure != nil {
					carrying++
				}
			}
			// What the writer printed and what the document holds, compared as
			// two numbers rather than as one.
			if parsed.Tests != len(cases) || parsed.Failures != carrying {
				t.Errorf("the attributes say tests=%d failures=%d and the document holds %d cases, %d failures:\n%s",
					parsed.Tests, parsed.Failures, len(cases), carrying, document)
			}
			if cases[0].Failure != nil {
				t.Errorf("the passing result carries a failure element: %+v", cases[0])
			}
			if cases[1].Failure == nil {
				t.Fatalf("the failing result carries no failure element: %+v", cases[1])
			}
			if !strings.Contains(cases[1].Failure.Body, payload) {
				t.Errorf("the detail did not survive as text inside its own element:\nwant %q\ngot  %q",
					payload, cases[1].Failure.Body)
			}
		})
	}
}

func TestMarkdownShowsEveryStepAndWhereItWasRead(t *testing.T) {
	rows := []check.DecisionRow{
		{
			Step: 1, Want: "ALLOW", Got: "ALLOW", ReasonCodes: []string{"RULE_ALLOW"},
			Source: "reports/r/evidence.jsonl:2", Outcome: assertion.Pass,
		},
		{
			Step: 3, Want: "DENY", Got: "no verdict",
			Source: "reports/r/evidence.jsonl", Outcome: assertion.Fail,
			Detail: "no proposed envelope carries context.stepId \"3\"",
		},
	}

	var out strings.Builder
	if err := report.WriteMarkdown(&out, mixed(), rows); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	page := out.String()

	for _, want := range []string{
		"flow-02-20260909T120000Z-a1b2c3d4",
		"flow-02-private-to-public-sink",
		"fail",
		"RULE_ALLOW",
		"reports/r/evidence.jsonl:2",
		"no verdict",
		"network-isolation/victim-unreachable",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the report does not mention %q:\n%s", want, page)
		}
	}
}

func TestMarkdownSaysWhenNoStepWasGraded(t *testing.T) {
	var out strings.Builder
	if err := report.WriteMarkdown(&out, mixed(), nil); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	if !strings.Contains(out.String(), "No step was graded") {
		t.Errorf("a report with no graded step does not say so:\n%s", out.String())
	}
}
