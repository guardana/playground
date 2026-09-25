package check

import (
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// runResult asserts that no event names a run, which is how the enforcer at its
// pin writes them. An event naming one came from something else, and a trail
// naming none is this run's only because the runner created its directory.
func (e Evidence) runResult(records assertion.Records) assertion.Result {
	result := assertion.Result{
		Check:  "evidence/run-id",
		Want:   "no event names a run, as the enforcer at its pin writes them",
		Source: e.EvidenceFile,
	}
	var named []string
	for i, event := range records.Evidence {
		if ids := runIDs(event); len(ids) > 0 {
			named = append(named, fmt.Sprintf("%s at line %d", strings.Join(ids, " and "), i+1))
		}
	}
	switch {
	case len(named) == 0 && !e.FreshTrail:
		result.Outcome = assertion.Indeterminate
		result.Got = fmt.Sprintf("%d events, none naming a run", len(records.Evidence))
		result.Detail = "no event names a run and the trail was not read from a directory this run created, " +
			"so nothing says this run wrote it"
		return result
	case len(named) == 0:
		result.Outcome = assertion.Pass
		result.Got = fmt.Sprintf("%d events, none naming a run, read from the directory this run created", len(records.Evidence))
		return result
	}
	result.Outcome = assertion.Fail
	result.Got = fmt.Sprintf("%d of %d events name a run", len(named), len(records.Evidence))
	result.Detail = "the trail carries " + strings.Join(named, ", ")
	return result
}
