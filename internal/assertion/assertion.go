// Package assertion is the interface every scenario check is written against.
//
// It holds one opinion, and everything else follows from it: the zero value of
// an outcome is indeterminate. A result nobody filled in, a check that returned
// nothing, a report with no results and a run whose services never started all
// arrive at the same answer, which is that nothing was established. Pass has to
// be written down by something that read a record.
//
// A check reads records: the evidence trail the enforcement plane wrote, the
// journals the victim tool servers kept, and which services actually came up.
// Nothing here reads what the agent said about its own work.
package assertion

import (
	"context"
	"time"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/journal"
)

// Outcome is what a check established. The zero value is Indeterminate.
type Outcome int

const (
	// Indeterminate is the zero value: nothing was established either way.
	Indeterminate Outcome = iota
	// Pass means a check read a record and the record said what was expected.
	Pass
	// Fail means a check read a record and the record said something else.
	Fail
)

// String names the outcome for a report a person reads.
func (o Outcome) String() string {
	switch o {
	case Indeterminate:
		return "indeterminate"
	case Pass:
		return "pass"
	case Fail:
		return "fail"
	default:
		return "unranked"
	}
}

// severity orders outcomes for Worse. An outcome this version cannot rank is
// worse than any it can: a value nobody understood must not carry a run to
// green.
func (o Outcome) severity() int {
	switch o {
	case Pass:
		return 0
	case Indeterminate:
		return 1
	case Fail:
		return 2
	default:
		return 3
	}
}

// Worse returns the outcome a report takes when it holds both.
func Worse(a, b Outcome) Outcome {
	if b.severity() > a.severity() {
		return b
	}
	return a
}

// Result is one check's conclusion and the record it read to reach it. Want and
// Got are for the person reading a red run; Source names the file and position
// so they can open it.
type Result struct {
	Check   string
	Outcome Outcome
	Want    string
	Got     string
	Source  string
	Detail  string
}

// Check reads records and reports what it established. It returns an error only
// when it could not read what it needed; that is not a failure of the thing
// under test, and Run records it as indeterminate rather than as a pass.
type Check interface {
	ID() string
	Run(ctx context.Context, records Records) ([]Result, error)
}

// Records is everything one run left behind.
type Records struct {
	RunID    string
	Scenario string
	Boot     Boot
	// Evidence is the whole trail of the run, in the order it was written. A
	// check that wants one request's trail groups it with evidence.ByRequest.
	Evidence []evidence.Event
	// Journals is what each victim tool server recorded, keyed by the name
	// compose gives the server.
	Journals map[string][]journal.Entry
}

// Service is one compose service and whether it came up.
type Service struct {
	Name    string
	Running bool
	Detail  string
}

// Boot is what the profile did when the runner brought it up. It exists so that
// "the service never started" is a record a check reads, rather than a silence
// a check mistakes for success.
type Boot struct {
	Profile  string
	Services []Service
}

// NotRunning names the services that did not come up.
func (b Boot) NotRunning() []string {
	var missing []string
	for _, service := range b.Services {
		if !service.Running {
			missing = append(missing, service.Name)
		}
	}
	return missing
}

// Complete reports whether the profile came up whole. A report naming no
// services observed nothing and is never complete.
func (b Boot) Complete() bool {
	return len(b.Services) > 0 && len(b.NotRunning()) == 0
}

// Report is one scenario's whole run.
type Report struct {
	Scenario  string
	RunID     string
	StartedAt time.Time
	EndedAt   time.Time
	Results   []Result
	// Gap is why the scenario is a named gap, empty for any other: its pass
	// means the system still does what its documentation says today.
	Gap string
}

// Suite names what a pass means: a catalogue scenario passes when the system
// does what it documents, a known gap when it still does what it documents
// today instead of what it should.
func (r Report) Suite() string {
	if r.Gap != "" {
		return "known-gap"
	}
	return "catalogue"
}

// Outcome is the worst result in the report. A report with no results is
// indeterminate: a run that checked nothing did not pass.
func (r Report) Outcome() Outcome {
	if len(r.Results) == 0 {
		return Indeterminate
	}
	worst := Pass
	for _, result := range r.Results {
		worst = Worse(worst, result.Outcome)
	}
	return worst
}

// Run executes every check in order and collects what each established.
//
// A check that returns an error, and a check that returns no results at all,
// each contribute one indeterminate result naming the check. Both are the same
// fact: this check did not look at anything, so nothing about it is known.
func Run(ctx context.Context, records Records, checks ...Check) Report {
	report := Report{
		Scenario:  records.Scenario,
		RunID:     records.RunID,
		StartedAt: time.Now(),
	}
	for _, check := range checks {
		results, err := check.Run(ctx, records)
		switch {
		case err != nil:
			report.Results = append(report.Results, Result{
				Check:   check.ID(),
				Outcome: Indeterminate,
				Detail:  "the check could not read what it needed: " + err.Error(),
			})
		case len(results) == 0:
			report.Results = append(report.Results, Result{
				Check:   check.ID(),
				Outcome: Indeterminate,
				Detail:  "the check reported nothing, so it established nothing",
			})
		default:
			report.Results = append(report.Results, results...)
		}
	}
	report.EndedAt = time.Now()
	return report
}
