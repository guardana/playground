package check

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
)

// uncomputed is the tag the enforcer stamps on a proposal it decided with no
// run's state, a call it gave no run.
const uncomputed = "flow.v1.state=uncomputed"

// runResult asserts how the enforcer at its pin names runs: every event on a
// request carries the run it minted, one run for the whole trail, except the
// events of a request whose proposal says it was decided with no run, and an
// envelope's own context names none, since nothing sets it through MCP. The
// id is the enforcer's, so the trail is this lab run's only because the runner
// created the directory it was read from.
func (e Evidence) runResult(records assertion.Records) assertion.Result {
	result := assertion.Result{
		Check: "evidence/run-id",
		Want: "every event on a request names the same one run, or none where its proposal was decided with no run, " +
			"and no envelope names a run of its own",
		Source: e.EvidenceFile,
	}
	run := planeRun(records.Evidence)
	wrong, stamped, unstamped := runsNamed(records.Evidence, run)
	got := fmt.Sprintf("%d events on requests naming run %q, %d on requests decided with no run", stamped, run, unstamped)
	switch {
	case len(wrong) > 0:
		result.Outcome = assertion.Fail
		result.Got = fmt.Sprintf("%d events off the trail's run %q", len(wrong), run)
		result.Detail = "the trail carries " + strings.Join(wrong, ", ")
	case stamped+unstamped == 0:
		result.Outcome, result.Got = assertion.Indeterminate, "no event on a request"
		result.Detail = "no event names a request, so the trail records no run"
	case !e.FreshTrail:
		result.Outcome, result.Got = assertion.Indeterminate, got
		result.Detail = "the trail was not read from a directory this run created, so nothing says this run wrote it"
	default:
		result.Outcome, result.Got = assertion.Pass, got+", read from the directory this run created"
	}
	return result
}

// runsNamed sorts the events on requests: those naming run, those of a request
// decided with no run naming none, and every other one, described.
func runsNamed(events []evidence.Event, run string) (wrong []string, stamped, unstamped int) {
	runless := runlessRequests(events)
	wrong = envelopeRuns(events)
	for i, event := range events {
		switch {
		case event.RequestID == "" && event.RunID != "" && event.RunID != run:
			wrong = append(wrong, fmt.Sprintf("%s at line %d", event.RunID, i+1))
		case event.RequestID == "":
		case event.RunID == "" && runless[event.RequestID]:
			unstamped++
		case event.RunID == "":
			wrong = append(wrong, fmt.Sprintf("no run at line %d", i+1))
		case event.RunID != run:
			wrong = append(wrong, fmt.Sprintf("%s at line %d", event.RunID, i+1))
		default:
			stamped++
		}
	}
	return wrong, stamped, unstamped
}

// envelopeRuns describes every envelope that names a run of its own.
func envelopeRuns(events []evidence.Event) []string {
	var named []string
	for i, event := range events {
		if event.Proposed != nil && event.Proposed.Context != nil && event.Proposed.Context.RunID != "" {
			named = append(named, fmt.Sprintf("an envelope naming %s at line %d", event.Proposed.Context.RunID, i+1))
		}
	}
	return named
}

// runlessRequests are the requests whose proposal carries the uncomputed tag.
func runlessRequests(events []evidence.Event) map[string]bool {
	found := map[string]bool{}
	for _, event := range events {
		if event.Kind == evidence.KindActionProposed && event.Proposed != nil && event.Proposed.Context != nil &&
			slices.Contains(event.Proposed.Context.Tags, uncomputed) {
			found[event.RequestID] = true
		}
	}
	return found
}
