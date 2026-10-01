package check

import (
	"strings"
	"testing"

	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// A step's trail already refuses a request closed twice; the result grader
// refuses it on its own as well, so it never reads a status from one of two.
func TestTheResultGraderRefusesTwoClosingRecords(t *testing.T) {
	closing := evidence.Event{Kind: evidence.KindActionFailed, RequestID: "r1",
		Result: &evidence.ActionResult{Status: "RESULT_STATUS_BLOCKED"}}
	detail, _ := gradeResult(labspec.ResultExpectation{Status: "BLOCKED"}, []evidence.Event{closing, closing}, "", "r1")
	if !strings.Contains(detail, "2 closing records") {
		t.Errorf("two closing records were graded: %q", detail)
	}
	if detail, _ := gradeResult(labspec.ResultExpectation{Status: "BLOCKED"}, []evidence.Event{closing}, "", "r1"); detail != "" {
		t.Errorf("one closing record with the stated status was refused: %q", detail)
	}
}

// The pairing already refuses a request proposed twice; the tag grader refuses
// it on its own as well, so it never reads the tags of one of two.
func TestTheTagGraderRefusesTwoProposals(t *testing.T) {
	proposed := evidence.Event{Kind: evidence.KindActionProposed, RequestID: "r1",
		Proposed: &evidence.ActionEnvelope{Context: &evidence.RunContext{Tags: []string{"flow.v1.untrusted=true"}}}}
	detail, _ := gradeTags([]string{"flow.v1.untrusted=true"}, []evidence.Event{proposed, proposed}, "", "r1")
	if !strings.Contains(detail, "2 "+string(evidence.KindActionProposed)) {
		t.Errorf("two proposals were graded: %q", detail)
	}
	if detail, _ := gradeTags([]string{"flow.v1.untrusted=true"}, []evidence.Event{proposed}, "", "r1"); detail != "" {
		t.Errorf("one proposal carrying the tag was refused: %q", detail)
	}
}
