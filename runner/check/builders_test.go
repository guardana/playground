package check_test

import (
	"strconv"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/internal/labspec"
)

// The builders below assemble a trail by hand rather than by running anything,
// because the checks are what decides whether the lab tells the truth and a
// check tested against its own producer would only prove the two agree.

const bundleDigest = "sha256:" +
	"0000000000000000000000000000000000000000000000000000000000000000"

// thisRun is the run the records name. A record stamped with anything else was
// written about something other than the run being graded.
const thisRun = "run-1"

// decided is one request's trail: proposed, decided, then blocked or completed.
// The links are the ones evidence.ValidateChain reads, so a trail built here is
// one the chain check accepts unless a test breaks it on purpose.
type decided struct {
	step int
	// runID stamps the events the way the gateway stamps them, on the event and
	// on the proposed envelope's context. It defaults to the run the records
	// name, so a test that wants a trail from another run says so.
	runID       string
	requestID   string
	verdict     string
	reasons     []string
	obligations []string
	digest      string
	preview     string
	blocked     bool
}

func (d decided) events() []evidence.Event {
	digest := d.digest
	if digest == "" {
		digest = bundleDigest
	}
	runID := d.runID
	if runID == "" {
		runID = thisRun
	}
	stepID := strconv.Itoa(d.step)
	at := time.Date(2026, 9, 9, 12, 0, d.step, 0, time.UTC)

	proposed := evidence.Event{
		EventID:    d.requestID + "-1",
		Kind:       evidence.KindActionProposed,
		RequestID:  d.requestID,
		RunID:      runID,
		OccurredAt: at,
		Proposed: &evidence.ActionEnvelope{
			RequestID: d.requestID,
			Context:   &evidence.RunContext{RunID: runID, StepID: stepID},
			Arguments: &evidence.Arguments{RedactedPreview: d.preview},
		},
	}
	decision := evidence.Event{
		EventID:     d.requestID + "-2",
		Kind:        evidence.KindPolicyDecided,
		RequestID:   d.requestID,
		RunID:       runID,
		OccurredAt:  at,
		PrevEventID: proposed.EventID,
		Decision: &evidence.Decision{
			RequestID:          d.requestID,
			PolicyBundleDigest: digest,
			Verdict:            "VERDICT_" + d.verdict,
			ReasonCodes:        d.reasons,
			Obligations:        obligations(d.obligations),
		},
	}
	if d.blocked {
		return []evidence.Event{proposed, decision, {
			EventID:     d.requestID + "-3",
			Kind:        evidence.KindActionBlocked,
			RequestID:   d.requestID,
			RunID:       runID,
			OccurredAt:  at,
			PrevEventID: decision.EventID,
		}}
	}
	started := evidence.Event{
		EventID:     d.requestID + "-3",
		Kind:        evidence.KindActionStarted,
		RequestID:   d.requestID,
		RunID:       runID,
		OccurredAt:  at,
		PrevEventID: decision.EventID,
	}
	return []evidence.Event{proposed, decision, started, {
		EventID:     d.requestID + "-4",
		Kind:        evidence.KindActionCompleted,
		RequestID:   d.requestID,
		RunID:       runID,
		OccurredAt:  at,
		PrevEventID: started.EventID,
	}}
}

func obligations(types []string) []evidence.Obligation {
	var built []evidence.Obligation
	for _, name := range types {
		built = append(built, evidence.Obligation{Type: name})
	}
	return built
}

func trail(parts ...decided) []evidence.Event {
	var events []evidence.Event
	for _, part := range parts {
		events = append(events, part.events()...)
	}
	return events
}

func records(events []evidence.Event, journals map[string][]journal.Entry) assertion.Records {
	return assertion.Records{
		RunID:    thisRun,
		Scenario: "scenario-1",
		Boot:     assertion.Boot{Profile: "core"},
		Evidence: events,
		Journals: journals,
	}
}

// served is one journal line for a call the victim answered, stamped with the
// run the records name.
func served(server, tool string) journal.Entry {
	return servedIn(thisRun, server, tool)
}

func servedIn(runID, server, tool string) journal.Entry {
	return journal.Entry{
		OccurredAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Server:     server,
		Tool:       tool,
		RunID:      runID,
		Status:     journal.Served,
	}
}

func scenario(decisions map[int]labspec.DecisionExpectation) labspec.Scenario {
	return labspec.Scenario{
		SchemaVersion:   labspec.SchemaVersion,
		ID:              "scenario-1",
		Title:           "a scenario built for a check test",
		Profile:         []string{"core"},
		EnforcementMode: "enforce",
		Trajectory:      "trajectories/scenario-1.yaml",
		Expect:          labspec.Expect{Decisions: decisions},
	}
}
