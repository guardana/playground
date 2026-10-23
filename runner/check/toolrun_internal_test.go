package check

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
)

const cleanRule = "lab.rule.clean"

// silentRun is a run that ran cleanRule and named every list empty.
func silentRun() record.Run {
	return record.Run{
		Target:     "target",
		RulesRun:   record.Named([]string{cleanRule}),
		Findings:   record.Named[record.Finding](nil),
		Waived:     record.Named[record.Finding](nil),
		Unverified: record.Named[record.Finding](nil),
		Errors:     record.Named[string](nil),
		Coverage:   record.Named[record.Gap](nil),
		Scope:      record.Named([]string{"target/model.pkl"}),
	}
}

func gradeRecord(t *testing.T, want labspec.VerifierExpectation, ran record.Run, check string) assertion.Result {
	t.Helper()
	for _, result := range gradeReport(VerifierRun{Step: 1, URL: "target"}, want, ran, nil) {
		if result.Check == "verifier/step-1/"+check {
			return result
		}
	}
	t.Fatalf("no result named verifier/step-1/%s", check)
	return assertion.Result{}
}

// A list the run left out is not one it named empty: a rule whose findings,
// waived findings, unverified results or errors went unreported has not been
// shown to have left nothing.
func TestARuleIsNotCleanBesideAListTheRunLeftOut(t *testing.T) {
	want := labspec.VerifierExpectation{FindingsExclude: []string{cleanRule}}
	if result := gradeRecord(t, want, silentRun(), "no-finding/"+cleanRule); result.Outcome != assertion.Pass {
		t.Fatalf("every list named empty: %s (%q), want pass", result.Outcome, result.Got)
	}
	for name, unreport := range map[string]func(*record.Run){
		"findings":   func(r *record.Run) { r.Findings = record.List[record.Finding]{} },
		"waived":     func(r *record.Run) { r.Waived = record.List[record.Finding]{} },
		"unverified": func(r *record.Run) { r.Unverified = record.List[record.Finding]{} },
		"errors":     func(r *record.Run) { r.Errors = record.List[string]{} },
	} {
		ran := silentRun()
		unreport(&ran)
		result := gradeRecord(t, want, ran, "no-finding/"+cleanRule)
		if result.Outcome != assertion.Fail || result.Got != "the report records no "+name+" list" {
			t.Errorf("%s not reported: %s (%q), want fail", name, result.Outcome, result.Got)
		}
	}
}

// A scan's waived list is exact; one the run left out says nothing, not none.
func TestAScanWithItsWaivedListLeftOutFails(t *testing.T) {
	scope := []string{"model.pkl"}
	want := labspec.VerifierExpectation{Scope: &scope, Findings: &[]labspec.FindingExpectation{}}
	if result := gradeRecord(t, want, silentRun(), "waived"); result.Outcome != assertion.Pass {
		t.Fatalf("waived named empty: %s (%q), want pass", result.Outcome, result.Got)
	}
	ran := silentRun()
	ran.Waived = record.List[record.Finding]{}
	if result := gradeRecord(t, want, ran, "waived"); result.Outcome != assertion.Fail {
		t.Errorf("waived not reported: %s (%q), want fail", result.Outcome, result.Got)
	}
}

// Findings and unverified results the run left out fail what a step asserts
// about them, as a scan and as a probe.
func TestAnUnreportedListFailsEveryCheckThatReadsIt(t *testing.T) {
	scope := []string{"model.pkl"}
	scan := labspec.VerifierExpectation{Scope: &scope, Findings: &[]labspec.FindingExpectation{}}
	probe := labspec.VerifierExpectation{
		FindingsInclude:   []labspec.FindingExpectation{{RuleID: cleanRule}},
		UnverifiedInclude: []string{cleanRule},
	}
	for _, test := range []struct {
		want   labspec.VerifierExpectation
		change func(*record.Run)
		check  string
	}{
		{scan, func(r *record.Run) { r.Findings = record.List[record.Finding]{} }, "findings"},
		{scan, func(r *record.Run) { r.Unverified = record.List[record.Finding]{} }, "unverified"},
		{probe, func(r *record.Run) { r.Findings = record.List[record.Finding]{} }, "finding/" + cleanRule},
		{probe, func(r *record.Run) { r.Unverified = record.List[record.Finding]{} }, "unverified/" + cleanRule},
	} {
		ran := silentRun()
		test.change(&ran)
		result := gradeRecord(t, test.want, ran, test.check)
		if result.Outcome != assertion.Fail || !strings.HasPrefix(result.Got, "the report records no ") {
			t.Errorf("%s with its list not reported: %s (%q), want fail", test.check, result.Outcome, result.Got)
		}
	}
}
