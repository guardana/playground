package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
)

// gradeScan adds a scan step's exact record: the files the verifier says it
// listed, every finding with the file it is about, and no unverified result or
// waived finding the step does not name. The verifier names a file under the
// target it was handed, so prefix is that target and a slash.
func gradeScan(want labspec.VerifierExpectation, report verifierReport, prefix string, add func(string, string, func() (bool, string))) {
	if want.Scope == nil || want.Findings == nil {
		return
	}
	add("scope", describeList("exactly", *want.Scope), func() (bool, string) { return report.scopeIs(*want.Scope, prefix) })
	add("findings", describeFindings(*want.Findings), func() (bool, string) { return report.findingsAre(*want.Findings, prefix) })
	add("unverified", describeList("only", want.UnverifiedInclude), func() (bool, string) {
		got := ruleIDs(report.Unverified)
		for _, rule := range got {
			if !slices.Contains(want.UnverifiedInclude, rule) {
				return false, describeList("reported", got)
			}
		}
		return true, describeList("reported", got)
	})
	add("waived", "none", func() (bool, string) {
		return len(report.Waived) == 0, describeList("reported", ruleIDs(report.Waived))
	})
}

func (r verifierReport) scopeIs(want []string, prefix string) (bool, string) {
	if r.Scope == nil {
		return false, "the report records no scope"
	}
	got := slices.Sorted(slices.Values(r.Scope.Files))
	expected := make([]string, 0, len(want))
	for _, file := range want {
		expected = append(expected, prefix+file)
	}
	slices.Sort(expected)
	return slices.Equal(got, expected), describeList("listed", got)
}

// findingsAre matches every reported finding to one expected finding and back:
// a finding the scenario does not name is one nobody expected.
func (r verifierReport) findingsAre(want []labspec.FindingExpectation, prefix string) (bool, string) {
	seen := make([]string, 0, len(r.Findings))
	for _, finding := range r.Findings {
		seen = append(seen, fmt.Sprintf("%s %s %s", finding.RuleID, finding.Severity, finding.TargetRef))
	}
	described := describeList("", seen)
	if len(r.Findings) != len(want) {
		return false, described
	}
	taken := make([]bool, len(r.Findings))
	for _, expected := range want {
		index := untakenFinding(r.Findings, taken, expected, prefix)
		if index < 0 {
			return false, described
		}
		taken[index] = true
	}
	return true, described
}

func describeFindings(want []labspec.FindingExpectation) string {
	named := make([]string, 0, len(want))
	for _, finding := range want {
		named = append(named, fmt.Sprintf("%s %s %s", finding.RuleID, finding.Severity, finding.File))
	}
	return describeList("exactly", named)
}

// untakenFinding is the first finding the expectation describes that no other
// has taken, or -1.
func untakenFinding(findings []verifierFinding, taken []bool, want labspec.FindingExpectation, prefix string) int {
	for i, finding := range findings {
		if !taken[i] && finding.RuleID == want.RuleID && finding.Severity == want.Severity &&
			refFile(finding.TargetRef) == prefix+want.File && strings.Contains(finding.summary(), want.SummaryContains) {
			return i
		}
	}
	return -1
}

// refFile is the file a finding's ref names: a finding on a line is
// file:line.
func refFile(ref string) string {
	at := strings.LastIndex(ref, ":")
	if at < 0 || at == len(ref)-1 || strings.Trim(ref[at+1:], "0123456789") != "" {
		return ref
	}
	return ref[:at]
}
