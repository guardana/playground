package check_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// plane builds events the way the enforcer at its pin writes them: request ids
// it minted, no run id, no step number, one project and tenant.
type plane struct {
	events []evidence.Event
	tools  []string
}

func (p *plane) add(request string, kind evidence.Kind, decision *evidence.Decision) *plane {
	prev := ""
	for _, event := range p.events {
		if event.RequestID == request {
			prev = event.EventID
		}
	}
	id := fmt.Sprintf("%s-%d", request, len(p.events))
	event := evidence.Event{
		EventID: id, Kind: kind, RequestID: request, ProjectID: thisProject, TenantID: thisTenant,
		OccurredAt: time.Date(2026, 9, 24, 12, 0, len(p.events), 0, time.UTC), PrevEventID: prev, Decision: decision,
	}
	if kind == evidence.KindActionProposed {
		event.Proposed = &evidence.ActionEnvelope{RequestID: request, Action: &evidence.Action{Name: p.proposing(), Protocol: "mcp"}}
	}
	p.events = append(p.events, event)
	return p
}

// proposing is the tool the next proposal names: the next of p.tools, or
// everyTool once they run out.
func (p *plane) proposing() string {
	if len(p.tools) == 0 {
		return everyTool
	}
	tool := p.tools[0]
	p.tools = p.tools[1:]
	return tool
}

func verdict(name string, codes ...string) *evidence.Decision {
	return &evidence.Decision{Verdict: "VERDICT_" + name, ReasonCodes: codes, PolicyBundleDigest: bundleDigest}
}

var fullHold = []string{"ACTION_PROPOSED", "POLICY_DECIDED", "APPROVAL_REQUESTED", "APPROVAL_DECIDED", "ACTION_STARTED", "ACTION_COMPLETED"}

// heldThenResumed is step 1 held, step 2 a pending retry that opens no trail,
// step 3 the retry that resumes the held trail, with a read at step 4 between.
func heldThenResumed() *plane {
	p := &plane{}
	p.add("req-held", evidence.KindActionProposed, nil).
		add("req-held", evidence.KindPolicyDecided, verdict("REQUIRE_APPROVAL", "RULE_REQUIRE_APPROVAL")).
		add("req-held", evidence.KindApprovalRequested, nil).
		add("req-held", evidence.KindApprovalDecided, nil).
		add("req-held", evidence.KindActionStarted, nil).
		add("req-held", evidence.KindActionCompleted, nil).
		add("req-read", evidence.KindActionProposed, nil).
		add("req-read", evidence.KindPolicyDecided, verdict("ALLOW", "RULE_ALLOW")).
		add("req-read", evidence.KindActionStarted, nil).
		add("req-read", evidence.KindActionCompleted, nil)
	return p
}

func heldSpec() labspec.Scenario {
	return scenario(map[int]labspec.DecisionExpectation{
		1: {Verdict: "REQUIRE_APPROVAL", ReasonCodesInclude: []string{"RULE_REQUIRE_APPROVAL"}},
		2: {Opens: labspec.OpensNone},
		3: {Resumes: 1, Trail: fullHold},
		4: {Verdict: "ALLOW"},
	})
}

func grade(t *testing.T, spec labspec.Scenario, events []evidence.Event) map[string]assertion.Result {
	t.Helper()
	return gradeAgainst(t, spec, sameTool(spec), events)
}

func gradeAgainst(
	t *testing.T, spec labspec.Scenario, trajectory labspec.Trajectory, events []evidence.Event,
) map[string]assertion.Result {
	t.Helper()
	graded := map[string]assertion.Result{}
	for _, subject := range []assertion.Check{
		check.Decisions{Scenario: spec, Trajectory: trajectory, EvidenceFile: evidenceFile},
		check.Trails{Scenario: spec, Trajectory: trajectory, EvidenceFile: evidenceFile},
	} {
		results, err := subject.Run(context.Background(), records(events, nil))
		if err != nil {
			t.Fatalf("%s: %v", subject.ID(), err)
		}
		for _, result := range results {
			graded[result.Check] = result
		}
	}
	return graded
}

func TestAControlShapedRunIsGradedByTheOrderItsTrailsOpened(t *testing.T) {
	graded := grade(t, heldSpec(), heldThenResumed().events)
	for _, name := range []string{"decisions/step-1", "decisions/step-2", "decisions/step-3", "decisions/step-4", "trails/opened"} {
		if graded[name].Outcome != assertion.Pass {
			t.Errorf("%s is %s: %s (got %s)", name, graded[name].Outcome, graded[name].Detail, graded[name].Got)
		}
	}
	if graded["decisions/step-4"].Source != evidenceFile+":8" {
		t.Errorf("step 4 was read from %s, want the read's decision at line 8", graded["decisions/step-4"].Source)
	}
}

func TestARetryThatOpenedATrailOfItsOwnIsCaught(t *testing.T) {
	p := heldThenResumed()
	p.add("req-retry", evidence.KindActionProposed, nil).
		add("req-retry", evidence.KindPolicyDecided, verdict("REQUIRE_APPROVAL"))
	graded := grade(t, heldSpec(), p.events)
	if graded["trails/opened"].Outcome != assertion.Fail || !strings.Contains(graded["trails/opened"].Detail, "req-retry") {
		t.Errorf("an unclaimed trail was %s: %s", graded["trails/opened"].Outcome, graded["trails/opened"].Detail)
	}
	if graded["decisions/step-2"].Outcome != assertion.Fail {
		t.Errorf("a step that opens none passed beside an unclaimed trail: %+v", graded["decisions/step-2"])
	}
}

func TestStepsPairWithTrailsInTheOrderTheyOpened(t *testing.T) {
	swapped := heldSpec()
	swapped.Expect.Decisions[4] = labspec.DecisionExpectation{Verdict: "REQUIRE_APPROVAL"}
	swapped.Expect.Decisions[1] = labspec.DecisionExpectation{Verdict: "ALLOW"}
	graded := grade(t, swapped, heldThenResumed().events)
	for _, name := range []string{"decisions/step-1", "decisions/step-4"} {
		if graded[name].Outcome != assertion.Fail {
			t.Errorf("%s passed on the other step's trail: %+v", name, graded[name])
		}
	}
}

func TestTheWholeTrailIsComparedKindByKind(t *testing.T) {
	spec := heldSpec()
	spec.Expect.Decisions[3] = labspec.DecisionExpectation{Resumes: 1, Trail: []string{
		"ACTION_PROPOSED", "POLICY_DECIDED", "APPROVAL_REQUESTED", "APPROVAL_EXPIRED", "ACTION_BLOCKED",
	}}
	graded := grade(t, spec, heldThenResumed().events)
	result := graded["decisions/step-3"]
	if result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "APPROVAL_DECIDED") {
		t.Errorf("a trail that ran was graded as one that expired: %+v", result)
	}
	failed := slicesClone(fullHold)
	failed[5] = "ACTION_FAILED"
	spec.Expect.Decisions[3] = labspec.DecisionExpectation{Resumes: 1, Trail: failed}
	if result := grade(t, spec, heldThenResumed().events)["decisions/step-3"]; result.Outcome != assertion.Fail {
		t.Errorf("a trail of the same length ending otherwise was %s", result.Outcome)
	}
}

func slicesClone(values []string) []string { return append([]string(nil), values...) }

func TestABlockIsReadFromItsOwnRecord(t *testing.T) {
	expired := func() *plane {
		p := &plane{}
		return p.add("req-held", evidence.KindActionProposed, nil).
			add("req-held", evidence.KindPolicyDecided, verdict("REQUIRE_APPROVAL")).
			add("req-held", evidence.KindApprovalRequested, nil).
			add("req-held", evidence.KindApprovalExpired, nil).
			add("req-held", evidence.KindActionBlocked, verdict("DENY", "APPROVAL_EXPIRED"))
	}
	want := labspec.DecisionExpectation{
		Verdict: "REQUIRE_APPROVAL",
		Blocked: &labspec.BlockExpectation{Verdict: "DENY", ReasonCodesInclude: []string{"APPROVAL_EXPIRED"}},
		Trail:   []string{"ACTION_PROPOSED", "POLICY_DECIDED", "APPROVAL_REQUESTED", "APPROVAL_EXPIRED", "ACTION_BLOCKED"},
	}
	graded := grade(t, scenario(map[int]labspec.DecisionExpectation{1: want}), expired().events)
	if graded["decisions/step-1"].Outcome != assertion.Pass {
		t.Fatalf("an expired hold was %s: %s", graded["decisions/step-1"].Outcome, graded["decisions/step-1"].Detail)
	}
	want.Blocked.Verdict = "INDETERMINATE"
	graded = grade(t, scenario(map[int]labspec.DecisionExpectation{1: want}), expired().events)
	if result := graded["decisions/step-1"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "INDETERMINATE") {
		t.Errorf("a block with the wrong verdict was %s: %s", result.Outcome, result.Detail)
	}
	want.Blocked.Verdict = "DENY"
	want.Blocked.ReasonCodesInclude = []string{"APPROVAL_REJECTED"}
	graded = grade(t, scenario(map[int]labspec.DecisionExpectation{1: want}), expired().events)
	if result := graded["decisions/step-1"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "APPROVAL_REJECTED") {
		t.Errorf("a block with the wrong reason code was %s: %s", result.Outcome, result.Detail)
	}
}

func TestAStepNumberTheTrailCarriesMustAgreeWithTheOrder(t *testing.T) {
	events := trail(decided{step: 2, requestID: "r1", verdict: "ALLOW"}, decided{step: 1, requestID: "r2", verdict: "ALLOW"})
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}, 2: {Verdict: "ALLOW"}})
	graded := grade(t, spec, events)
	if result := graded["decisions/step-1"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "says it is step 2") {
		t.Errorf("a trail stamped step 2 was graded as step 1: %+v", result)
	}
}
