package check

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
)

// The report and pin versions this lab reads. A report of another version may
// name the same fields for other things, so it is not read under this one's
// rules.
const (
	verifierReportSchema = 18
	verifierPinSchema    = 2
	// maxVerifierFile bounds one report or pin read into memory.
	maxVerifierFile = 8 << 20
)

// verifierReport is the part of the verifier's `--format json` report a
// scenario is graded from. Gate, Shortfall and Errors are pointers because a
// report that leaves them out has not said "none".
type verifierReport struct {
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
			Shortfall *[]verifierShortfall `json:"shortfall"`
		} `json:"coverage"`
	} `json:"run"`
	Findings   []verifierFinding `json:"findings"`
	Unverified []verifierFinding `json:"unverified"`
	Waived     []verifierFinding `json:"waived"`
	Errors     *[]verifierError  `json:"errors"`
	// Scope is what a scan listed; null for any other run.
	Scope *struct {
		Files []string `json:"files"`
	} `json:"scope"`
}

// verifierShortfall is coverage the run needed and did not get, by kind and
// what it concerned.
type verifierShortfall struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// verifierError is a check that could not run; Source is the rule id, entry
// point or file that failed.
type verifierError struct {
	Source string `json:"source"`
}

type verifierFinding struct {
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	// TargetRef is the file a scan finding is about, as the scan names it.
	TargetRef string `json:"target_ref"`
	Evidence  *struct {
		Summary string `json:"summary"`
	} `json:"evidence"`
}

func (f verifierFinding) summary() string {
	if f.Evidence == nil {
		return ""
	}
	return f.Evidence.Summary
}

// verifierPin is the approved manifest a write_pin step leaves.
type verifierPin struct {
	SchemaVersion int               `json:"schema_version"`
	Server        string            `json:"server"`
	Tools         map[string]string `json:"tools"`
}

// reportLists says whether a report named each list a check reads as exact:
// a list left out, or null, has not said "none".
type reportLists struct {
	Findings   *json.RawMessage `json:"findings"`
	Unverified *json.RawMessage `json:"unverified"`
	Waived     *json.RawMessage `json:"waived"`
}

// readVerifierFile reads a file the verifier wrote into a directory it can
// write, so a link or an endless file there is refused rather than followed,
// and decodes the one read into each value.
func readVerifierFile(path string, values ...any) error {
	body, err := runfile.ReadRegular(path, maxVerifierFile)
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

// readReport reads a report and holds it to the endpoint the step probed: a
// report about another target is a record of something else.
func readReport(path, url string) (verifierReport, error) {
	var report verifierReport
	var lists reportLists
	if err := readVerifierFile(path, &report, &lists); err != nil {
		return verifierReport{}, fmt.Errorf("the verifier's report cannot be read: %w", err)
	}
	switch {
	case lists.Findings == nil || lists.Unverified == nil || lists.Waived == nil:
		return verifierReport{}, fmt.Errorf("the report leaves out findings, unverified or waived, which is not saying none")
	case report.SchemaVersion != verifierReportSchema:
		return verifierReport{}, fmt.Errorf("the report is schema %d and the lab reads %d",
			report.SchemaVersion, verifierReportSchema)
	case report.Run.Target.Ref != url:
		return verifierReport{}, fmt.Errorf("the report is about %q, not the probed %s", report.Run.Target.Ref, url)
	case len(report.Run.ResultSummary.RulesRun) == 0:
		return verifierReport{}, fmt.Errorf("the report names no rule that ran")
	}
	return report, nil
}

func readPin(path, url string) (verifierPin, error) {
	var pin verifierPin
	if err := readVerifierFile(path, &pin); err != nil {
		return verifierPin{}, fmt.Errorf("the pin cannot be read: %w", err)
	}
	switch {
	case pin.SchemaVersion != verifierPinSchema:
		return verifierPin{}, fmt.Errorf("the pin is schema %d and the lab reads %d", pin.SchemaVersion, verifierPinSchema)
	case pin.Server != url:
		return verifierPin{}, fmt.Errorf("the pin approves %q, not the probed %s", pin.Server, url)
	case len(pin.Tools) == 0:
		return verifierPin{}, fmt.Errorf("the pin approves no tool")
	}
	return pin, nil
}

func ruleIDs(findings []verifierFinding) []string {
	ids := make([]string, 0, len(findings))
	for _, finding := range findings {
		ids = append(ids, finding.RuleID)
	}
	return ids
}

// concludedClean reports whether a rule ran and left nothing: no finding, no
// waived finding, no unverified result and no error. A rule that never ran, ran
// and could not tell, or found something a waiver accepted, has not established
// that the thing it looks for is absent.
func (r verifierReport) concludedClean(rule string) (bool, string) {
	switch {
	case !slices.Contains(r.Run.ResultSummary.RulesRun, rule):
		return false, "the rule did not run"
	case slices.Contains(ruleIDs(r.Findings), rule):
		return false, "the rule reported a finding"
	case slices.Contains(ruleIDs(r.Waived), rule):
		return false, "the rule reported a finding, and it was waived"
	case slices.Contains(ruleIDs(r.Unverified), rule):
		return false, "the rule ran and could not reach a verdict"
	case slices.ContainsFunc(deref(r.Errors), func(e verifierError) bool { return e.Source == rule }):
		return false, "the rule raised an error"
	}
	return true, "the rule ran and reported nothing"
}

// hasFinding reports whether a finding matches everything the expectation
// states, and otherwise what the findings of that rule say.
func (r verifierReport) hasFinding(want labspec.FindingExpectation) (bool, string) {
	var seen []string
	for _, finding := range r.Findings {
		if finding.RuleID != want.RuleID {
			continue
		}
		seen = append(seen, fmt.Sprintf("%s %q", finding.Severity, finding.summary()))
		if (want.Severity == "" || finding.Severity == want.Severity) &&
			strings.Contains(finding.summary(), want.SummaryContains) {
			return true, fmt.Sprintf("%s: %s %q", finding.RuleID, finding.Severity, finding.summary())
		}
	}
	if len(seen) == 0 {
		return false, "no finding of " + want.RuleID
	}
	return false, strings.Join(seen, "; ")
}
