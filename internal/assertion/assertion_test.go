package assertion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

// The zero value has to be the outcome that establishes nothing. A struct
// somebody forgot to fill in must not read as a pass.
func TestZeroOutcomeIsIndeterminate(t *testing.T) {
	var outcome assertion.Outcome
	if outcome != assertion.Indeterminate {
		t.Fatalf("zero outcome = %v, want Indeterminate", outcome)
	}
	var result assertion.Result
	if result.Outcome != assertion.Indeterminate {
		t.Fatalf("zero result outcome = %v, want Indeterminate", result.Outcome)
	}
}

func TestWorseRanksFailAboveIndeterminateAbovePass(t *testing.T) {
	cases := []struct{ a, b, want assertion.Outcome }{
		{assertion.Pass, assertion.Pass, assertion.Pass},
		{assertion.Pass, assertion.Indeterminate, assertion.Indeterminate},
		{assertion.Indeterminate, assertion.Pass, assertion.Indeterminate},
		{assertion.Indeterminate, assertion.Fail, assertion.Fail},
		{assertion.Fail, assertion.Pass, assertion.Fail},
	}
	for _, test := range cases {
		if got := assertion.Worse(test.a, test.b); got != test.want {
			t.Errorf("Worse(%v, %v) = %v, want %v", test.a, test.b, got, test.want)
		}
	}
}

// An outcome from a version this one does not rank is worse than any it does.
// Ranking it low would let a value nobody understood carry a run to green.
func TestWorseRanksAnUnknownOutcomeWorst(t *testing.T) {
	unknown := assertion.Outcome(99)
	if got := assertion.Worse(unknown, assertion.Fail); got != unknown {
		t.Errorf("Worse(unknown, Fail) = %v, want the unknown outcome", got)
	}
}

type check struct {
	id      string
	results []assertion.Result
	err     error
}

func (c check) ID() string { return c.id }
func (c check) Run(context.Context, assertion.Records) ([]assertion.Result, error) {
	return c.results, c.err
}

func TestReportTakesTheWorstResult(t *testing.T) {
	report := assertion.Run(t.Context(), assertion.Records{},
		check{id: "a", results: []assertion.Result{{Check: "a", Outcome: assertion.Pass}}},
		check{id: "b", results: []assertion.Result{{Check: "b", Outcome: assertion.Fail}}},
	)
	if got := report.Outcome(); got != assertion.Fail {
		t.Errorf("report outcome = %v, want Fail", got)
	}
}

// A run that checked nothing has established nothing, whatever the exit code of
// the thing it was watching.
func TestReportWithNoResultsIsIndeterminate(t *testing.T) {
	report := assertion.Run(t.Context(), assertion.Records{})
	if got := report.Outcome(); got != assertion.Indeterminate {
		t.Errorf("empty report outcome = %v, want Indeterminate", got)
	}
}

// A check that returns no results looked at nothing, and saying so is the
// difference between a check that passed and a check that never ran.
func TestACheckThatReturnsNothingIsRecordedAsIndeterminate(t *testing.T) {
	report := assertion.Run(t.Context(), assertion.Records{}, check{id: "silent"})
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	if report.Results[0].Outcome != assertion.Indeterminate {
		t.Errorf("outcome = %v, want Indeterminate", report.Results[0].Outcome)
	}
	if report.Results[0].Check != "silent" {
		t.Errorf("check = %q, want silent", report.Results[0].Check)
	}
}

// A check that could not read what it needed did not establish that the thing
// is fine. It reports indeterminate and carries the reason.
func TestACheckThatFailsToRunIsIndeterminateAndKeepsTheReason(t *testing.T) {
	report := assertion.Run(t.Context(), assertion.Records{},
		check{id: "unreadable", err: errors.New("evidence.jsonl: no such file")})
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	if report.Results[0].Outcome != assertion.Indeterminate {
		t.Errorf("outcome = %v, want Indeterminate", report.Results[0].Outcome)
	}
	if report.Results[0].Detail == "" {
		t.Error("the reason the check could not run was dropped")
	}
	if got := report.Outcome(); got != assertion.Indeterminate {
		t.Errorf("report outcome = %v, want Indeterminate", got)
	}
}

func TestRunKeepsCheckOrder(t *testing.T) {
	report := assertion.Run(t.Context(), assertion.Records{},
		check{id: "first", results: []assertion.Result{{Check: "first", Outcome: assertion.Pass}}},
		check{id: "second", results: []assertion.Result{{Check: "second", Outcome: assertion.Pass}}},
	)
	if len(report.Results) != 2 || report.Results[0].Check != "first" {
		t.Fatalf("results = %+v", report.Results)
	}
}

// A service that did not start is the case the runner exists to refuse to call
// ok. Boot names it, so a check can read it rather than infer it from silence.
func TestBootNamesWhatDidNotStart(t *testing.T) {
	boot := assertion.Boot{Profile: "core", Services: []assertion.Service{
		{Name: "victim-fs", Running: true},
		{Name: "victim-mail", Running: false, Detail: "exited with code 1"},
	}}
	missing := boot.NotRunning()
	if len(missing) != 1 || missing[0] != "victim-mail" {
		t.Fatalf("NotRunning = %v, want [victim-mail]", missing)
	}
}

// A boot report that lists no services observed nothing. Treating that as "all
// well" is the false green the whole runner is built to avoid.
func TestBootWithNoServicesIsNotAWorkingProfile(t *testing.T) {
	if (assertion.Boot{Profile: "core"}).Complete() {
		t.Fatal("a boot report naming no services reported itself complete")
	}
}

func TestOutcomeStringsAreStable(t *testing.T) {
	for outcome, want := range map[assertion.Outcome]string{
		assertion.Pass:          "pass",
		assertion.Fail:          "fail",
		assertion.Indeterminate: "indeterminate",
	} {
		if got := outcome.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", outcome, got, want)
		}
	}
}
