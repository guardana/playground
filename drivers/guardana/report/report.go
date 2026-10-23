// Package report reads what the verifier leaves behind: the JSON report a step
// prints and the pin a write_pin step approves. Each is handed on as a neutral
// record, so nothing that grades a run reads the verifier's own formats.
package report

import (
	"encoding/json"
	"fmt"

	"github.com/guardana/playground/internal/record"
	"github.com/guardana/playground/internal/runfile"
)

// The report and pin versions this lab reads. A report of another version may
// name the same fields for other things, so it is not read under this one's
// rules.
const (
	reportSchema = 18
	pinSchema    = 2
	// maxFile bounds one report or pin read into memory.
	maxFile = 8 << 20
)

// runReport is the part of the verifier's `--format json` report a scenario is
// graded from. Gate, Shortfall and Errors are pointers because a report that
// leaves them out has not said "none".
type runReport struct {
	SchemaVersion int `json:"schema_version"`
	Run           struct {
		Target struct {
			Ref string `json:"ref"`
		} `json:"target"`
		ResultSummary struct {
			RulesRun []string `json:"rules_run"`
			Gate     *string  `json:"gate"`
		} `json:"result_summary"`
		Coverage struct {
			Shortfall *[]shortfall `json:"shortfall"`
		} `json:"coverage"`
	} `json:"run"`
	Findings   []finding `json:"findings"`
	Unverified []finding `json:"unverified"`
	Waived     []finding `json:"waived"`
	Errors     *[]failed `json:"errors"`
	// Scope is what a scan listed; null for any other run.
	Scope *struct {
		Files []string `json:"files"`
	} `json:"scope"`
}

// shortfall is coverage the run needed and did not get, by kind and what it
// concerned.
type shortfall struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// failed is a check that could not run; Source is the rule id, entry point or
// file that failed.
type failed struct {
	Source string `json:"source"`
}

type finding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	// TargetRef is the file a scan finding is about, as the scan names it.
	TargetRef string `json:"target_ref"`
	Evidence  *struct {
		Summary string `json:"summary"`
	} `json:"evidence"`
}

func (f finding) summary() string {
	if f.Evidence == nil {
		return ""
	}
	return f.Evidence.Summary
}

// reportLists says whether a report named each list a check reads as exact:
// a list left out, or null, has not said "none".
type reportLists struct {
	Findings   *json.RawMessage `json:"findings"`
	Unverified *json.RawMessage `json:"unverified"`
	Waived     *json.RawMessage `json:"waived"`
}

// readFile reads a file the verifier wrote into a directory it can write, so
// a link or an endless file there is refused rather than followed, and decodes
// the one read into each value.
func readFile(path string, values ...any) error {
	body, err := runfile.ReadRegular(path, maxFile)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	for _, value := range values {
		if err := json.Unmarshal(body, value); err != nil {
			return err
		}
	}
	return nil
}

// Read reads a report and holds it to the endpoint the step probed: a report
// about another target is a record of something else.
func Read(path, url string) (record.Run, error) {
	var report runReport
	var lists reportLists
	if err := readFile(path, &report, &lists); err != nil {
		return record.Run{}, fmt.Errorf("the verifier's report cannot be read: %w", err)
	}
	switch {
	case lists.Findings == nil || lists.Unverified == nil || lists.Waived == nil:
		return record.Run{}, fmt.Errorf("the report leaves out findings, unverified or waived, which is not saying none")
	case report.SchemaVersion != reportSchema:
		return record.Run{}, fmt.Errorf("the report is schema %d and the lab reads %d",
			report.SchemaVersion, reportSchema)
	case report.Run.Target.Ref != url:
		return record.Run{}, fmt.Errorf("the report is about %q, not the probed %s", report.Run.Target.Ref, url)
	case len(report.Run.ResultSummary.RulesRun) == 0:
		return record.Run{}, fmt.Errorf("the report names no rule that ran")
	}
	return report.toRecord(lists), nil
}

// toRecord is the report as a neutral record. A list is reported only when the
// report named it; every value is carried as the report wrote it.
func (r runReport) toRecord(lists reportLists) record.Run {
	run := record.Run{
		Target:     r.Run.Target.Ref,
		Verdict:    r.Run.ResultSummary.Gate,
		RulesRun:   record.List[string]{Items: r.Run.ResultSummary.RulesRun, Reported: r.Run.ResultSummary.RulesRun != nil},
		Findings:   toFindings(r.Findings, lists.Findings != nil),
		Waived:     toFindings(r.Waived, lists.Waived != nil),
		Unverified: toFindings(r.Unverified, lists.Unverified != nil),
	}
	if r.Errors != nil {
		run.Errors = record.Named(make([]string, 0, len(*r.Errors)))
		for _, failed := range *r.Errors {
			run.Errors.Items = append(run.Errors.Items, failed.Source)
		}
	}
	if r.Run.Coverage.Shortfall != nil {
		run.Coverage = record.Named(make([]record.Gap, 0, len(*r.Run.Coverage.Shortfall)))
		for _, gap := range *r.Run.Coverage.Shortfall {
			run.Coverage.Items = append(run.Coverage.Items, record.Gap{Kind: gap.Kind, Name: gap.Name})
		}
	}
	if r.Scope != nil {
		run.Scope = record.Named(r.Scope.Files)
	}
	return run
}

func toFindings(findings []finding, named bool) record.List[record.Finding] {
	list := record.List[record.Finding]{Reported: named}
	for _, finding := range findings {
		list.Items = append(list.Items, record.Finding{
			Rule: finding.RuleID, Severity: finding.Severity, Target: finding.TargetRef, Summary: finding.summary(),
		})
	}
	return list
}
