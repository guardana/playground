package check

import (
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// runResult reports whether the trail was written for the run being graded.
//
// A complete, coherent, digest-carrying trail from an earlier run satisfies
// every other assertion here, so without this one a run is graded on whatever
// was on disk. Nothing but a fresh run directory stands between the two, and a
// directory is luck rather than an assertion.
func (e Evidence) runResult(records assertion.Records) assertion.Result {
	if e.Unstamped {
		return e.unstampedResult(records)
	}
	result := assertion.Result{
		Check:  "evidence/run-id",
		Want:   "every event stamped with run " + records.RunID,
		Source: e.EvidenceFile,
	}
	stamped, elsewhere, first := 0, []string{}, 0
	for i, event := range records.Evidence {
		named := runIDs(event)
		if len(named) == 0 {
			continue
		}
		stamped++
		if writtenForRun(named, records.RunID) {
			continue
		}
		if first == 0 {
			first = i + 1
		}
		elsewhere = append(elsewhere, fmt.Sprintf("%s at line %d", strings.Join(named, " and "), i+1))
	}
	switch {
	case stamped == 0:
		result.Outcome = assertion.Indeterminate
		result.Got = "no event names a run"
		result.Detail = "nothing in the trail says which run wrote it"
	case len(elsewhere) > 0:
		result.Outcome = assertion.Fail
		result.Got = fmt.Sprintf("%d of %d events name another run", len(elsewhere), stamped)
		result.Source = fmt.Sprintf("%s:%d", e.EvidenceFile, first)
		result.Detail = fmt.Sprintf("this run is %s and the trail carries %s",
			records.RunID, strings.Join(elsewhere, ", "))
	default:
		result.Outcome = assertion.Pass
		result.Got = fmt.Sprintf("%d events, every one stamped %s", stamped, records.RunID)
	}
	return result
}

// unstampedResult asserts that no event names a run, which is how the enforcer
// at its pin writes them. An event naming one came from something else, and a
// trail naming none is this run's only because the runner created its directory.
func (e Evidence) unstampedResult(records assertion.Records) assertion.Result {
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
