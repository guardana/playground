package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

// auth grades the authentication decisions a token-gated victim recorded in
// this run: every decision the scenario names has to be among them. They are
// the victim's own record of what a request carried, so whether the run's
// token reached it is read from the far side, not from the verifier's report.
func (e Effects) auth(victim string, records assertion.Records) (assertion.Result, bool) {
	want := e.Scenario.Expect.Effects[victim].AuthInclude
	if len(want) == 0 {
		return assertion.Result{}, false
	}
	result := assertion.Result{
		Check:  "effects/" + victim + "/auth",
		Want:   describeAuth(want),
		Source: journal.File(e.JournalDir, victim),
	}
	decisions, collected := records.Auth[victim]
	if !collected {
		result.Outcome = assertion.Fail
		result.Got, result.Detail = uncollected(records, victim, "decided")
		return result, true
	}
	mine := slices.DeleteFunc(slices.Clone(decisions), func(d journal.Auth) bool { return d.RunID != records.RunID })
	seen := make([]string, 0, len(mine))
	for _, decision := range mine {
		seen = append(seen, fmt.Sprintf("%s/%s/%d", decision.Credential, decision.Session, decision.Status))
	}
	slices.Sort(seen)
	result.Got = "decided " + strings.Join(slices.Compact(seen), ", ")
	if len(mine) == 0 {
		result.Got = "decided nothing in this run"
	}
	var missing []string
	for _, expected := range want {
		if !slices.ContainsFunc(mine, expected.Matches) {
			missing = append(missing, describeAuth([]labspec.AuthExpectation{expected}))
		}
	}
	result.Outcome = assertion.Pass
	if len(missing) > 0 {
		result.Outcome, result.Detail = assertion.Fail, "never recorded: "+strings.Join(missing, "; ")
	}
	return result, true
}

func describeAuth(want []labspec.AuthExpectation) string {
	parts := make([]string, 0, len(want))
	for _, expected := range want {
		part := "credential " + expected.Credential
		if expected.Session != "" {
			part += ", session " + expected.Session
		}
		if expected.Status != 0 {
			part += fmt.Sprintf(", status %d", expected.Status)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
