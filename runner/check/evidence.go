package check

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// Evidence grades the trail itself, apart from what it says about any one step.
//
// Every assertion here is over a set of records, and a set that turns out to be
// empty is reported as indeterminate rather than satisfied. "No envelope
// carries a preview" is true of a trail with no envelopes in it, and reading
// that as a pass would let a run that recorded nothing report that the privacy
// default held.
type Evidence struct {
	Scenario     labspec.Scenario
	EvidenceFile string
	// ReadError is why the trail could not be read, when it could not be. A
	// trail nobody could read and a trail with nothing in it are two facts, and
	// the second one sends a reader to a file that turns out to be full.
	ReadError error
	// FreshTrail is set by the runner when the trail file lives in the run
	// directory it has just created. The enforcer writes no run id, so without
	// this nothing says the trail is this run's.
	FreshTrail bool
}

// ID names the check in a report.
func (Evidence) ID() string { return "evidence" }

// Run grades the trail. chain_complete and policy_digest_present are asserted
// only when the scenario asks for them; content_captured is asserted either
// way, because false is the assertion that the privacy default held.
func (e Evidence) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	if trail, nothingToGrade := e.trailResult(records); nothingToGrade {
		return []assertion.Result{trail}, nil
	}
	if e.Scenario.Expect.Evidence == nil {
		return []assertion.Result{{
			Check: "evidence/expectation", Outcome: assertion.Indeterminate,
			Want: "an evidence expectation", Got: "the scenario states none", Source: e.EvidenceFile,
		}}, nil
	}
	expect := *e.Scenario.Expect.Evidence
	results := []assertion.Result{e.runResult(records)}
	if expect.ChainComplete {
		results = append(results, e.chainResults(records.Evidence)...)
	}
	if expect.PolicyDigestPresent {
		results = append(results, e.digestResult(records.Evidence))
	}
	return append(results, e.captureResult(records.Evidence, expect.ContentCaptured)), nil
}

// trailResult reports on the file itself when there was nothing in it to grade,
// and false when there is a trail to read.
//
// A trail nobody could read and a trail with nothing in it are two facts. The
// first is indeterminate and carries the reason: the reader could not get at
// the record, which is not a failure of the thing under test, and reporting it
// as an empty trail sends a person to a file that turns out to be full. The
// second is a failure — the enforcement plane recorded no decision, and unknown
// is never a pass.
func (e Evidence) trailResult(records assertion.Records) (assertion.Result, bool) {
	result := assertion.Result{
		Check:  "evidence/trail",
		Want:   "a trail to read",
		Source: e.EvidenceFile,
	}
	switch {
	case e.ReadError != nil:
		result.Outcome = assertion.Indeterminate
		result.Got = "the trail could not be read"
		result.Detail = e.ReadError.Error()
	case len(records.Evidence) == 0:
		result.Outcome = assertion.Fail
		result.Got = "no events"
		result.Detail = "the trail holds no event, so the enforcement plane recorded no decision"
	default:
		return assertion.Result{}, false
	}
	return result, true
}

// requestless are the kinds that belong to no request: an operator reloading a
// bundle, a detector reporting after the fact.
var requestless = []evidence.Kind{evidence.KindPolicyReloaded, evidence.KindFindingRaised}

// chainResults grades one trail per request, in request order. Events with no
// request id are graded apart: they are no request's chain.
func (e Evidence) chainResults(events []evidence.Event) []assertion.Result {
	trails := evidence.ByRequest(events)
	results := make([]assertion.Result, 0, len(trails))
	if unscoped, present := trails[""]; present {
		results = append(results, e.requestlessResult(unscoped))
		delete(trails, "")
	}
	for _, requestID := range slices.Sorted(maps.Keys(trails)) {
		result := assertion.Result{
			Check:  "evidence/chain-complete/" + requestID,
			Want:   "one coherent trail for the request",
			Source: e.EvidenceFile,
		}
		switch err := evidence.ValidateChain(trails[requestID]); {
		case err == nil:
			result.Outcome = assertion.Pass
			result.Got = fmt.Sprintf("%d events in the documented order", len(trails[requestID]))
		case errors.Is(err, evidence.ErrChainIndeterminate):
			// A kind this reader cannot place is not a defect in the producer,
			// and merging it with a broken link would report a newer producer
			// as a broken one.
			result.Outcome = assertion.Indeterminate
			result.Got = err.Error()
		default:
			result.Outcome = assertion.Fail
			result.Got = err.Error()
		}
		results = append(results, result)
	}
	return results
}

func (e Evidence) requestlessResult(unscoped []evidence.Event) assertion.Result {
	result := assertion.Result{
		Check:  "evidence/requestless",
		Want:   "only a reload or a finding carries no request id",
		Source: e.EvidenceFile,
	}
	for _, event := range unscoped {
		if !slices.Contains(requestless, event.Kind) {
			result.Outcome = assertion.Fail
			result.Got = fmt.Sprintf("event %q is %s and names no request", event.EventID, event.Kind)
			return result
		}
	}
	result.Outcome = assertion.Pass
	result.Got = fmt.Sprintf("%d events, each a reload or a finding", len(unscoped))
	return result
}

func (e Evidence) digestResult(events []evidence.Event) assertion.Result {
	result := assertion.Result{
		Check:  "evidence/policy-digest-present",
		Want:   "every " + string(evidence.KindPolicyDecided) + " carries a policyBundleDigest",
		Source: e.EvidenceFile,
	}
	decided, missing := 0, []string{}
	for i, event := range events {
		if event.Kind != evidence.KindPolicyDecided || event.Decision == nil {
			continue
		}
		decided++
		if strings.TrimSpace(event.Decision.PolicyBundleDigest) == "" {
			missing = append(missing, fmt.Sprintf("line %d", i+1))
		}
	}
	switch {
	case decided == 0:
		result.Outcome = assertion.Indeterminate
		result.Got = "no decision to read a digest from"
	case len(missing) > 0:
		result.Outcome = assertion.Fail
		result.Got = fmt.Sprintf("%d of %d decisions carry none", len(missing), decided)
		result.Source = e.EvidenceFile + ":" + strings.TrimPrefix(missing[0], "line ")
		result.Detail = "no policyBundleDigest at " + strings.Join(missing, ", ")
	default:
		result.Outcome = assertion.Pass
		result.Got = fmt.Sprintf("%d of %d decisions carry one", decided, decided)
	}
	return result
}

// captureResult grades arguments.redactedPreview across every proposed
// envelope. False is the assertion that no envelope carries text, which is the
// privacy default; true is the assertion that at least one does.
func (e Evidence) captureResult(events []evidence.Event, captured bool) assertion.Result {
	result := assertion.Result{
		Check:  "evidence/content-captured",
		Want:   fmt.Sprintf("arguments.redactedPreview present: %t", captured),
		Source: e.EvidenceFile,
	}
	proposed, carrying := 0, []int{}
	for i, event := range events {
		if event.Kind != evidence.KindActionProposed || event.Proposed == nil {
			continue
		}
		proposed++
		if event.Proposed.Arguments != nil && event.Proposed.Arguments.RedactedPreview != "" {
			carrying = append(carrying, i+1)
		}
	}
	if proposed == 0 {
		result.Outcome = assertion.Indeterminate
		result.Got = "no proposed envelope to read arguments from"
		return result
	}
	result.Got = fmt.Sprintf("%d of %d envelopes carry a preview", len(carrying), proposed)
	if (len(carrying) > 0) == captured {
		result.Outcome = assertion.Pass
		return result
	}
	result.Outcome = assertion.Fail
	if len(carrying) > 0 {
		result.Source = fmt.Sprintf("%s:%d", e.EvidenceFile, carrying[0])
		result.Detail = "the scenario expects no captured content and the trail carries some"
		return result
	}
	result.Detail = "the scenario expects captured content and the trail carries none"
	return result
}
