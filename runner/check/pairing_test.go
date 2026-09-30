package check_test

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// Step 1 opened nothing and step 2, stated to open none, opened a trail: the
// counts balance, and only the tool each proposal names tells the shift.
func TestAMissingOpeningAndAnExtraOneDoNotCancelOut(t *testing.T) {
	p := &plane{tools: []string{"web.fetch", "mail.send"}}
	p.add("req-from-step2", evidence.KindActionProposed, nil).
		add("req-from-step2", evidence.KindPolicyDecided, verdict("DENY", "RULE_DENY")).
		add("req-from-step3", evidence.KindActionProposed, nil).
		add("req-from-step3", evidence.KindPolicyDecided, verdict("DENY", "RULE_DENY"))
	spec := scenario(map[int]labspec.DecisionExpectation{
		1: {Verdict: "DENY"},
		2: {Opens: labspec.OpensNone},
		3: {Verdict: "DENY"},
	})
	graded := gradeAgainst(t, spec, calls("fs.read", "web.fetch", "mail.send"), p.events)
	step := graded["decisions/step-1"]
	if step.Outcome != assertion.Fail || !strings.Contains(step.Detail, "fs.read") || !strings.Contains(step.Detail, "web.fetch") {
		t.Errorf("step 1 on step 2's trail was %s: %s", step.Outcome, step.Detail)
	}
	if opened := graded["trails/opened"]; opened.Outcome != assertion.Fail || !strings.Contains(opened.Detail, "web.fetch") {
		t.Errorf("trails/opened was %s: %s", opened.Outcome, opened.Detail)
	}
	if third := graded["decisions/step-3"]; third.Outcome != assertion.Pass {
		t.Errorf("step 3 on its own trail was %s: %s", third.Outcome, third.Detail)
	}
}

func TestAProposalNamingNoToolFailsItsStep(t *testing.T) {
	p := &plane{}
	p.add("r1", evidence.KindActionProposed, nil).add("r1", evidence.KindPolicyDecided, verdict("ALLOW"))
	p.events[0].Proposed.Action = nil
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	if step := grade(t, spec, p.events)["decisions/step-1"]; step.Outcome != assertion.Fail || !strings.Contains(step.Detail, "no tool") {
		t.Errorf("a proposal naming no tool was %s: %s", step.Outcome, step.Detail)
	}
}

// One request proposed twice is one trail opened twice: its step fails, and
// the count of trails is not thrown off by it.
func TestASecondProposalOnOneRequestFailsItsStep(t *testing.T) {
	p := &plane{}
	p.add("r1", evidence.KindActionProposed, nil).
		add("r1", evidence.KindPolicyDecided, verdict("ALLOW")).
		add("r1", evidence.KindActionProposed, nil)
	spec := scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}})
	graded := grade(t, spec, p.events)
	if step := graded["decisions/step-1"]; step.Outcome != assertion.Fail || !strings.Contains(step.Detail, "2 "+string(evidence.KindActionProposed)) {
		t.Errorf("a request proposed twice was %s: %s", step.Outcome, step.Detail)
	}
	if opened := graded["trails/opened"]; opened.Outcome != assertion.Pass {
		t.Errorf("one request proposed twice counted as %s: %s", opened.Outcome, opened.Detail)
	}
}

// A trail another run opened is no opening of the trail's run: it neither
// takes a step's place in the order nor counts as an unclaimed trail.
func TestAnotherRunsOpeningIsNotPairedWithAStep(t *testing.T) {
	events := trail(
		decided{step: 1, requestID: "r1", verdict: "ALLOW"},
		decided{step: 2, requestID: "r0", verdict: "DENY", runID: "run-yesterday", blocked: true},
	)
	graded := grade(t, scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}}), events)
	if step := graded["decisions/step-1"]; step.Outcome != assertion.Pass || step.Source != evidenceFile+":2" {
		t.Errorf("step 1 was %s from %s: %s", step.Outcome, step.Source, step.Detail)
	}
	if opened := graded["trails/opened"]; opened.Outcome != assertion.Pass {
		t.Errorf("another run's opening counted: %s %s", opened.Outcome, opened.Detail)
	}
}

func TestADecisionAnotherRunWroteOnTheSameRequestIsNotRead(t *testing.T) {
	events := trail(decided{step: 1, requestID: "r1", verdict: "ALLOW"})
	foreign := events[1]
	foreign.EventID, foreign.RunID = "r1-yesterday", "run-yesterday"
	foreign.Decision = &evidence.Decision{Verdict: "VERDICT_DENY", PolicyBundleDigest: bundleDigest}
	events = append(events, foreign)
	step := grade(t, scenario(map[int]labspec.DecisionExpectation{1: {Verdict: "ALLOW"}}), events)["decisions/step-1"]
	if step.Outcome != assertion.Pass {
		t.Errorf("step 1 was %s: %s", step.Outcome, step.Detail)
	}
}

func TestTwoBlocksOnOneRequestAreNoOneBlock(t *testing.T) {
	p := &plane{}
	p.add("r1", evidence.KindActionProposed, nil).
		add("r1", evidence.KindPolicyDecided, verdict("ALLOW")).
		add("r1", evidence.KindActionBlocked, verdict("DENY", "APPROVAL_EXPIRED")).
		add("r1", evidence.KindActionBlocked, verdict("DENY", "APPROVAL_EXPIRED"))
	want := labspec.DecisionExpectation{Verdict: "ALLOW", Blocked: &labspec.BlockExpectation{Verdict: "DENY"}}
	step := grade(t, scenario(map[int]labspec.DecisionExpectation{1: want}), p.events)["decisions/step-1"]
	if step.Outcome != assertion.Fail || !strings.Contains(step.Detail, "2 "+string(evidence.KindActionBlocked)) {
		t.Errorf("two blocks on one request were %s: %s", step.Outcome, step.Detail)
	}
}

// The file holds a trail in the order it was delivered; its links give the
// order it was written in, and that is the one graded.
func TestATrailIsReadInTheOrderItsLinksGive(t *testing.T) {
	events := heldThenResumed().events
	events[2], events[3] = events[3], events[2]
	step := grade(t, heldSpec(), events)["decisions/step-3"]
	if step.Outcome != assertion.Pass {
		t.Errorf("a trail delivered out of order was %s: %s", step.Outcome, step.Detail)
	}
}

// A resuming step stating a verdict would re-read the opening step's
// POLICY_DECIDED and pass whether or not the hold resumed. The file format
// refuses it; the check does not read it either.
func TestAResumingStepIsNotGradedOnTheOpeningStepsVerdict(t *testing.T) {
	p := &plane{}
	p.add("req-held", evidence.KindActionProposed, nil).
		add("req-held", evidence.KindPolicyDecided, verdict("REQUIRE_APPROVAL")).
		add("req-held", evidence.KindApprovalRequested, nil)
	spec := scenario(map[int]labspec.DecisionExpectation{
		1: {Verdict: "REQUIRE_APPROVAL"},
		2: {Resumes: 1, Verdict: "REQUIRE_APPROVAL"},
	})
	if step := grade(t, spec, p.events)["decisions/step-2"]; step.Outcome != assertion.Fail {
		t.Errorf("a resume graded on the held verdict was %s: %s", step.Outcome, step.Detail)
	}
}
