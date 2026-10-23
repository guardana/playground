package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
)

// unreported is what a check reads beside a list the run left out: it has not
// said "none".
func unreported(list string) string {
	return "the report records no " + list + " list"
}

func ruleIDs(findings []record.Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, finding := range findings {
		ids = append(ids, finding.Rule)
	}
	return ids
}

// concludedClean reports whether a rule ran and left nothing: no finding, no
// waived finding, no unverified result and no error. A rule that never ran, ran
// and could not tell, or found something a waiver accepted, has not established
// that the thing it looks for is absent; nor has one beside a list the run
// left out.
func concludedClean(run record.Run, rule string) (bool, string) {
	switch {
	case !slices.Contains(run.RulesRun.Items, rule):
		return false, "the rule did not run"
	case !run.Findings.Reported:
		return false, unreported("findings")
	case !run.Waived.Reported:
		return false, unreported("waived")
	case !run.Unverified.Reported:
		return false, unreported("unverified")
	case !run.Errors.Reported:
		return false, unreported("errors")
	case slices.Contains(ruleIDs(run.Findings.Items), rule):
		return false, "the rule reported a finding"
	case slices.Contains(ruleIDs(run.Waived.Items), rule):
		return false, "the rule reported a finding, and it was waived"
	case slices.Contains(ruleIDs(run.Unverified.Items), rule):
		return false, "the rule ran and could not reach a verdict"
	case slices.Contains(run.Errors.Items, rule):
		return false, "the rule raised an error"
	}
	return true, "the rule ran and reported nothing"
}

// hasFinding reports whether a finding matches everything the expectation
// states, and otherwise what the findings of that rule say.
func hasFinding(run record.Run, want labspec.FindingExpectation) (bool, string) {
	if !run.Findings.Reported {
		return false, unreported("findings")
	}
	var seen []string
	for _, finding := range run.Findings.Items {
		if finding.Rule != want.RuleID {
			continue
		}
		seen = append(seen, fmt.Sprintf("%s %q", finding.Severity, finding.Summary))
		if (want.Severity == "" || finding.Severity == want.Severity) &&
			strings.Contains(finding.Summary, want.SummaryContains) {
			return true, fmt.Sprintf("%s: %s %q", finding.Rule, finding.Severity, finding.Summary)
		}
	}
	if len(seen) == 0 {
		return false, "no finding of " + want.RuleID
	}
	return false, strings.Join(seen, "; ")
}

// hasUnverified reports whether the rule's result is among those the run could
// not settle.
func hasUnverified(run record.Run, rule string) (bool, string) {
	if !run.Unverified.Reported {
		return false, unreported("unverified")
	}
	if slices.Contains(ruleIDs(run.Unverified.Items), rule) {
		return true, "reported unverified"
	}
	return false, "not among the unverified results"
}
