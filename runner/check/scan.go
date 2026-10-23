package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
)

// gradeScan adds a scan step's exact record: the files the verifier says it
// listed, every finding with the file it is about, and no unverified result or
// waived finding the step does not name. The verifier names a file under the
// target it was handed, so prefix is that target and a slash.
func gradeScan(want labspec.VerifierExpectation, ran record.Run, prefix string, add func(string, string, func() (bool, string))) {
	if want.Scope == nil || want.Findings == nil {
		return
	}
	add("scope", describeList("exactly", *want.Scope), func() (bool, string) { return scopeIs(ran, *want.Scope, prefix) })
	add("findings", describeFindings(*want.Findings), func() (bool, string) { return findingsAre(ran, *want.Findings, prefix) })
	add("unverified", describeList("only", want.UnverifiedInclude), func() (bool, string) {
		if !ran.Unverified.Reported {
			return false, unreported("unverified")
		}
		got := ruleIDs(ran.Unverified.Items)
		for _, rule := range got {
			if !slices.Contains(want.UnverifiedInclude, rule) {
				return false, describeList("reported", got)
			}
		}
		return true, describeList("reported", got)
	})
	add("waived", "none", func() (bool, string) {
		if !ran.Waived.Reported {
			return false, unreported("waived")
		}
		return len(ran.Waived.Items) == 0, describeList("reported", ruleIDs(ran.Waived.Items))
	})
}

func scopeIs(ran record.Run, want []string, prefix string) (bool, string) {
	if !ran.Scope.Reported {
		return false, "the report records no scope"
	}
	got := slices.Sorted(slices.Values(ran.Scope.Items))
	expected := make([]string, 0, len(want))
	for _, file := range want {
		expected = append(expected, prefix+file)
	}
	slices.Sort(expected)
	return slices.Equal(got, expected), describeList("listed", got)
}

// findingsAre matches every reported finding to one expected finding and back:
// a finding the scenario does not name is one nobody expected.
func findingsAre(ran record.Run, want []labspec.FindingExpectation, prefix string) (bool, string) {
	if !ran.Findings.Reported {
		return false, unreported("findings")
	}
	findings := ran.Findings.Items
	seen := make([]string, 0, len(findings))
	for _, finding := range findings {
		seen = append(seen, fmt.Sprintf("%s %s %s", finding.Rule, finding.Severity, finding.Target))
	}
	described := describeList("", seen)
	if len(findings) != len(want) {
		return false, described
	}
	taken := make([]bool, len(findings))
	for _, expected := range want {
		index := untakenFinding(findings, taken, expected, prefix)
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
func untakenFinding(findings []record.Finding, taken []bool, want labspec.FindingExpectation, prefix string) int {
	for i, finding := range findings {
		if !taken[i] && finding.Rule == want.RuleID && finding.Severity == want.Severity &&
			refFile(finding.Target) == prefix+want.File && strings.Contains(finding.Summary, want.SummaryContains) {
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
