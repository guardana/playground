package check_test

import (
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// A second call to the slowed victim whose result carries no times is not a
// call the fault can be read on, and passing over it would grade the fault on
// the calls that happened to be timed.
func TestACallToTheFaultedVictimWithoutTimesFails(t *testing.T) {
	graded, records := oneCallTo(1600 * time.Millisecond)
	if results := chaosOutcomes(t, graded, records); results[0].Outcome != assertion.Pass {
		t.Fatalf("the one timed call did not pass: %s", results[0].Got)
	}
	graded.Trajectory.Steps = append(graded.Trajectory.Steps, labspec.Step{Call: labspec.Call{Server: "victim-fs", Tool: "fs.list"}})
	graded.Scenario.Expect.Decisions[2] = labspec.DecisionExpectation{Verdict: "ALLOW"}
	records.Evidence = append(records.Evidence,
		evidence.Event{EventID: "e3", Kind: evidence.KindActionProposed, RequestID: "r2", Proposed: &evidence.ActionEnvelope{
			RequestID: "r2", Action: &evidence.Action{Name: "fs.list"}}},
		evidence.Event{EventID: "e4", Kind: evidence.KindActionCompleted, RequestID: "r2", PrevEventID: "e3",
			Result: &evidence.ActionResult{}},
	)
	results := chaosOutcomes(t, graded, records)
	if results[0].Outcome != assertion.Fail || !strings.Contains(results[0].Got, "step 2's call to victim-fs closed without exactly one timed result") {
		t.Errorf("an untimed second call was %s: %s", results[0].Outcome, results[0].Got)
	}
}
