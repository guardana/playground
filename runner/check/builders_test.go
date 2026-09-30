package check_test

import (
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

// thisRun is the lab's run the records name, which the victims stamp their
// journals with. planeRun is the run the enforcer minted for that lab run and
// stamps on every event of its trails.
const (
	thisRun  = "run-1"
	planeRun = "01PLANERUN"
)

// The scope every trail is written in: the contract chains events per request
// within one project and tenant.
const (
	thisProject = "project-1"
	thisTenant  = "tenant-1"
)

// decided is one request's trail: proposed, decided, then blocked or completed.
// The links are the ones evidence.ValidateChain reads, so a trail built here is
// one the chain check accepts unless a test breaks it on purpose.
type decided struct {
	step int
	// runID is the run the events name; empty is planeRun. A test sets it to
	// put a trail from another process in front of the checks.
	runID       string
	requestID   string
	verdict     string
	reasons     []string
	obligations []string
	digest      string
	preview     string
	blocked     bool
	// tool is the tool the proposal names; empty is the one sameTool calls.
	tool string
}

func (d decided) events() []evidence.Event {
	digest := d.digest
	if digest == "" {
		digest = bundleDigest
	}
	runID := d.runID
	if runID == "" {
		runID = planeRun
	}
	tool := d.tool
	if tool == "" {
		tool = everyTool
	}
	at := time.Date(2026, 9, 9, 12, 0, d.step, 0, time.UTC)

	proposed := evidence.Event{
		EventID:    d.requestID + "-1",
		Kind:       evidence.KindActionProposed,
		RequestID:  d.requestID,
		RunID:      runID,
		ProjectID:  thisProject,
		TenantID:   thisTenant,
		OccurredAt: at,
		Proposed: &evidence.ActionEnvelope{
			RequestID: d.requestID,
			Action:    &evidence.Action{Name: tool, Protocol: "mcp"},
			Context:   &evidence.RunContext{Tags: computedFlow()},
			Arguments: &evidence.Arguments{RedactedPreview: d.preview},
		},
	}
	decision := evidence.Event{
		EventID:     d.requestID + "-2",
		Kind:        evidence.KindPolicyDecided,
		RequestID:   d.requestID,
		RunID:       runID,
		ProjectID:   thisProject,
		TenantID:    thisTenant,
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
			ProjectID:   thisProject,
			TenantID:    thisTenant,
			OccurredAt:  at,
			PrevEventID: decision.EventID,
		}}
	}
	started := evidence.Event{
		EventID:     d.requestID + "-3",
		Kind:        evidence.KindActionStarted,
		RequestID:   d.requestID,
		RunID:       runID,
		ProjectID:   thisProject,
		TenantID:    thisTenant,
		OccurredAt:  at,
		PrevEventID: decision.EventID,
	}
	return []evidence.Event{proposed, decision, started, {
		EventID:     d.requestID + "-4",
		Kind:        evidence.KindActionCompleted,
		RequestID:   d.requestID,
		RunID:       runID,
		ProjectID:   thisProject,
		TenantID:    thisTenant,
		OccurredAt:  at,
		PrevEventID: started.EventID,
	}}
}

// computedFlow is the flow state the enforcer stamps on a proposal it decided
// within a run.
func computedFlow() []string {
	return []string{"flow.v1.untrusted=false", "flow.v1.max_read=PUBLIC"}
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

// everyTool is the tool sameTool calls at every step, and the one a proposal
// names unless a test says otherwise.
const everyTool = "fs.read"

// calls is a trajectory whose n-th step calls the n-th tool.
func calls(tools ...string) labspec.Trajectory {
	trajectory := labspec.Trajectory{SchemaVersion: labspec.SchemaVersion}
	for _, tool := range tools {
		trajectory.Steps = append(trajectory.Steps, labspec.Step{Call: labspec.Call{Server: "victim-fs", Tool: tool}})
	}
	return trajectory
}

// sameTool is a trajectory as long as the scenario's last graded step, calling
// everyTool at each.
func sameTool(spec labspec.Scenario) labspec.Trajectory {
	last := 0
	for step := range spec.Expect.Decisions {
		last = max(last, step)
	}
	tools := make([]string, last)
	for i := range tools {
		tools[i] = everyTool
	}
	return calls(tools...)
}

func scenario(decisions map[int]labspec.DecisionExpectation) labspec.Scenario {
	return labspec.Scenario{
		SchemaVersion:   labspec.SchemaVersion,
		ID:              "scenario-1",
		Title:           "a scenario built for a check test",
		Profile:         []string{"core"},
		EnforcementMode: "enforce",
		Trajectory:      "trajectories/scenario-1.yaml",
		Expect:          labspec.Expect{Decisions: decisions, Evidence: &labspec.EvidenceExpectation{}},
	}
}
