package check

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// opening is one trail this run opened: its first ACTION_PROPOSED, by line,
// and how many proposals the request carries in all.
type opening struct {
	requestID string
	line      int
	tool      string
	proposals int
}

// pairing ties each step that opens a trail to the trail it opened. The
// enforcer mints its own request ids and writes no step number, so the n-th
// trail opened in the run is the n-th step that opens one. A trail the
// trajectory did not open, or a step whose trail never opened, shifts every
// pairing after it; the tool each proposal names shows the shift when a missing
// trail and an extra one for another tool leave the counts equal. Two calls to
// the same tool cannot be told apart this way (docs/lab-files.md).
type pairing struct {
	// run is the enforcer's run the pairing reads (planeRun).
	run      string
	byStep   map[int]opening
	openers  []int
	openings []opening
	tools    []string
}

func pairTrails(spec labspec.Scenario, trajectory labspec.Trajectory, events []evidence.Event) pairing {
	p := pairing{run: planeRun(events), byStep: map[int]opening{}}
	for _, step := range trajectory.Steps {
		p.tools = append(p.tools, step.Call.Tool)
	}
	for _, step := range slices.Sorted(maps.Keys(spec.Expect.Decisions)) {
		if spec.Expect.Decisions[step].OpensTrail() {
			p.openers = append(p.openers, step)
		}
	}
	first := map[string]int{}
	for i, event := range events {
		if event.Kind != evidence.KindActionProposed || !writtenForRun(event, p.run) {
			continue
		}
		if at, seen := first[event.RequestID]; seen {
			p.openings[at].proposals++
			continue
		}
		first[event.RequestID] = len(p.openings)
		p.openings = append(p.openings, openingOf(event, i+1))
	}
	for i, step := range p.openers {
		if i < len(p.openings) {
			p.byStep[step] = p.openings[i]
		}
	}
	return p
}

func openingOf(event evidence.Event, line int) opening {
	found := opening{requestID: event.RequestID, line: line, proposals: 1}
	if proposed := event.Proposed; proposed != nil && proposed.Action != nil {
		found.tool = proposed.Action.Name
	}
	return found
}

func (p pairing) balanced() bool { return len(p.openings) == len(p.openers) }

func (p pairing) counts() string {
	return fmt.Sprintf("%d steps open a trail and the run opened %d", len(p.openers), len(p.openings))
}

// trailOf is the trail a step is graded on: its own, or the held one it resumes.
// The defect is empty when the trail was found and agrees with the step.
func (p pairing) trailOf(step int, want labspec.DecisionExpectation) (opening, string) {
	owner := step
	if want.Resumes != 0 {
		owner = want.Resumes
	}
	found, paired := p.byStep[owner]
	switch {
	case !paired:
		return opening{}, fmt.Sprintf("step %d's trail was never opened: %s", owner, p.counts())
	case found.proposals > 1:
		return found, fmt.Sprintf("request %q carries %d %s events, so its trail was opened more than once",
			found.requestID, found.proposals, evidence.KindActionProposed)
	}
	return found, p.toolDefect(owner, found)
}

// toolDefect is empty when the trail's proposal names the tool the step calls.
func (p pairing) toolDefect(step int, found opening) string {
	if step < 1 || step > len(p.tools) {
		return fmt.Sprintf("the trajectory has no step %d to name the tool its trail proposes", step)
	}
	switch want := p.tools[step-1]; {
	case found.tool == "":
		return fmt.Sprintf("step %d calls %s and the trail at line %d it is paired with names no tool",
			step, want, found.line)
	case found.tool != want:
		return fmt.Sprintf("step %d calls %s and the trail at line %d it is paired with proposes %s",
			step, want, found.line, found.tool)
	}
	return ""
}

// Trails asserts the run opened exactly one trail per step that opens one, each
// proposing the tool its step calls. An opening no step claims is a call the
// trajectory did not make, or a step that opened two, and it shifts every
// pairing after it.
type Trails struct {
	Scenario     labspec.Scenario
	Trajectory   labspec.Trajectory
	EvidenceFile string
}

// ID names the check in a report.
func (Trails) ID() string { return "trails" }

// Run reports one result: the openings against the steps that open one.
func (t Trails) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	paired := pairTrails(t.Scenario, t.Trajectory, records.Evidence)
	result := assertion.Result{
		Check:   "trails/opened",
		Want:    fmt.Sprintf("%d trails, one per step that opens one, each proposing its step's tool", len(paired.openers)),
		Got:     fmt.Sprintf("%d opened", len(paired.openings)),
		Source:  t.EvidenceFile,
		Outcome: assertion.Fail,
	}
	var defects []string
	if len(paired.openings) > len(paired.openers) {
		var unclaimed []string
		for _, found := range paired.openings[len(paired.openers):] {
			unclaimed = append(unclaimed, fmt.Sprintf("%q at line %d", found.requestID, found.line))
		}
		defects = append(defects, "no step claims "+strings.Join(unclaimed, ", "))
	}
	if !paired.balanced() {
		defects = append(defects, paired.counts())
	}
	for _, step := range paired.openers {
		if found, ok := paired.byStep[step]; ok {
			if defect := paired.toolDefect(step, found); defect != "" {
				defects = append(defects, defect)
			}
		}
	}
	if len(defects) == 0 {
		result.Outcome = assertion.Pass
	}
	result.Detail = strings.Join(defects, "; ")
	return []assertion.Result{result}, nil
}

// kindsOf is the request's trail as short kinds, in the order its links give,
// and why that order could not be read.
func kindsOf(events []evidence.Event, requestID string) ([]string, error) {
	var mine []evidence.Event
	for _, event := range events {
		if event.RequestID == requestID {
			mine = append(mine, event)
		}
	}
	ordered, err := evidence.OrderChain(mine)
	if err != nil {
		return nil, err
	}
	kinds := make([]string, 0, len(ordered))
	for _, event := range ordered {
		kinds = append(kinds, labspec.ShortKind(string(event.Kind)))
	}
	return kinds, nil
}

// eventsOn finds the events of one kind on one request, by index.
func eventsOn(events []evidence.Event, run, requestID string, kind evidence.Kind) []int {
	var found []int
	for i, event := range events {
		if event.Kind != kind || event.RequestID != requestID || !writtenForRun(event, run) {
			continue
		}
		found = append(found, i)
	}
	return found
}
