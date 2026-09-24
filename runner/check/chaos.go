package check

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/evidence"
	"github.com/guardana/playground/internal/labspec"
)

// ChaosFault is what the runner did about one fault of the scenario and what
// it read back. Held is set only from a record: the proxy listing the toxic,
// the enforcer's /healthz showing a backlog while the collector was down, the
// victim's second listing describing a tool otherwise than the snapshot the
// enforcer was classified from. A fault that was applied and never read back
// was not shown to be in place. Detail says what was read.
type ChaosFault struct {
	Name string
	// Victim and Latency name a latency toxic's path, which the trail has to
	// show was slower by at least the latency.
	Victim  string
	Latency time.Duration
	// Hang holds a hang on Victim to the trail: each call to it closed timed
	// out, after CallTimeout, the scenario's upstream.call_timeout.
	Hang        bool
	CallTimeout time.Duration
	Applied     bool
	Held        bool
	Lifted      bool
	// Unliftable is a fault nothing undoes: a listing, once made, stays made.
	Unliftable bool
	Detail     string
}

// hangSlack is how far past the call timeout a hung call may close: the
// enforcer stamps the result around the bounded call, not at the bound.
const hangSlack = time.Second

// timedOut is the status the enforcer's result carries for an upstream call
// its call timeout cut off.
const timedOut = "RESULT_STATUS_TIMEOUT"

// Chaos grades the faults a scenario named: each applied, read back in
// place, and lifted before the drain.
type Chaos struct {
	Scenario   labspec.Scenario
	Trajectory labspec.Trajectory
	Faults     []ChaosFault
	Source     string
}

// ID names the check in a report.
func (Chaos) ID() string { return "chaos" }

// Run reports one result per fault; a scenario naming faults the runner
// recorded nothing about is indeterminate.
func (c Chaos) Run(_ context.Context, records assertion.Records) ([]assertion.Result, error) {
	if len(c.Faults) == 0 || len(c.Faults) != len(c.Scenario.Chaos) {
		return []assertion.Result{{
			Check: "chaos/applied", Outcome: assertion.Indeterminate, Source: c.Source,
			Want: fmt.Sprintf("%d faults applied and lifted", len(c.Scenario.Chaos)),
			Got:  fmt.Sprintf("the runner recorded %d", len(c.Faults)),
		}}, nil
	}
	results := make([]assertion.Result, 0, len(c.Faults))
	for i, fault := range c.Faults {
		result := assertion.Result{
			Check: fmt.Sprintf("chaos/fault-%d", i+1), Want: wanted(fault),
			Source: c.Source, Outcome: assertion.Fail,
		}
		defects, read := faultDefects(fault), fault.Detail
		if fault.Latency > 0 || fault.Hang {
			calls, untimed := c.timedCalls(fault.Victim, records)
			read = strings.Join(append([]string{read}, trailRead(calls)...), "; ")
			for _, step := range untimed {
				defects = append(defects, fmt.Sprintf("step %d's call to %s closed without exactly one timed result", step, fault.Victim))
			}
			if fault.Hang {
				defects = append(defects, hangDefects(fault, calls)...)
			} else {
				defects = append(defects, latencyDefects(fault, calls)...)
			}
		}
		result.Got = "read: " + spoken(read)
		if len(defects) > 0 {
			result.Got = strings.Join(defects, "; ") + "; " + result.Got
		} else {
			result.Outcome = assertion.Pass
		}
		results = append(results, result)
	}
	return results, nil
}

func wanted(fault ChaosFault) string {
	if fault.Unliftable {
		return fault.Name + ": applied and read back in place; lifting does not apply, nothing undoes it"
	}
	return fault.Name + ": applied, read back in place, lifted"
}

func faultDefects(fault ChaosFault) []string {
	var defects []string
	for _, step := range []struct {
		done bool
		not  string
	}{
		{fault.Applied, "not applied"}, {fault.Held, "not read back in place"}, {fault.Lifted || fault.Unliftable, "not lifted"},
	} {
		if !step.done {
			defects = append(defects, step.not)
		}
	}
	return defects
}

// latencyDefects holds a latency toxic to the trail: every call to its victim
// that ran took at least the latency between the result's start and end, and
// at least one such call ran. A proxy the enforcer never went through slows
// nothing, and a scenario would pass on a fault that was never in the path.
func latencyDefects(fault ChaosFault, calls []timedCall) []string {
	var defects []string
	for _, call := range calls {
		if took := call.took(); took < fault.Latency {
			defects = append(defects, fmt.Sprintf("step %d's call to %s took %s, under the latency", call.step, fault.Victim, took))
		}
	}
	if len(calls) == 0 {
		defects = append(defects, "no call to "+fault.Victim+" closed with a timed result, so nothing went through the toxic")
	}
	return defects
}

// hangDefects holds a hang to the trail: every call to its victim closed timed
// out, no sooner than the call timeout and within hangSlack after it, and at
// least one did. A call that failed any other way, or was cut off by another
// bound, is not the timeout the scenario is about.
func hangDefects(fault ChaosFault, calls []timedCall) []string {
	if fault.CallTimeout <= 0 {
		return []string{"the scenario's gateway configuration sets no upstream.call_timeout to hold the hang to"}
	}
	var defects []string
	for _, call := range calls {
		took := call.took()
		switch {
		case call.result.Status != timedOut:
			defects = append(defects, fmt.Sprintf("step %d's call to %s closed %s, not %s", call.step, fault.Victim, spoken(call.result.Status), timedOut))
		case took < fault.CallTimeout || took >= fault.CallTimeout+hangSlack:
			defects = append(defects, fmt.Sprintf("step %d's call to %s took %s, not %s to %s",
				call.step, fault.Victim, took, fault.CallTimeout, fault.CallTimeout+hangSlack))
		}
	}
	if len(calls) == 0 {
		defects = append(defects, "no call to "+fault.Victim+" closed with a timed result, so nothing was held")
	}
	return defects
}

// timedCall is one step's call to a victim and the result its trail closed
// with, which carries both a start and an end.
type timedCall struct {
	step   int
	result *evidence.ActionResult
}

func (t timedCall) took() time.Duration { return t.result.EndedAt.Sub(*t.result.StartedAt) }

// trailRead says how each timed call closed, as the report's record of what
// the trail showed.
func trailRead(calls []timedCall) []string {
	read := make([]string, 0, len(calls))
	for _, call := range calls {
		read = append(read, fmt.Sprintf("the trail closed step %d %s after %s", call.step, spoken(call.result.Status), call.took()))
	}
	return read
}

// timedCalls returns each step's call to victim that closed with one result
// carrying a start and an end, and the steps whose call closed any other way:
// two closings, or a result without its times. A call that never closed,
// blocked before it ran, is neither.
func (c Chaos) timedCalls(victim string, records assertion.Records) ([]timedCall, []int) {
	paired := pairTrails(c.Scenario, c.Trajectory, records.Evidence, records.RunID)
	var calls []timedCall
	var untimed []int
	for step, found := range paired.byStep {
		if step > len(c.Trajectory.Steps) || c.Trajectory.Steps[step-1].Call.Server != victim {
			continue
		}
		closing := append(eventsOn(records.Evidence, records.RunID, found.requestID, evidence.KindActionCompleted),
			eventsOn(records.Evidence, records.RunID, found.requestID, evidence.KindActionFailed)...)
		if len(closing) == 0 {
			continue
		}
		result := records.Evidence[closing[0]].Result
		if len(closing) != 1 || result == nil || result.StartedAt == nil || result.EndedAt == nil {
			untimed = append(untimed, step)
			continue
		}
		calls = append(calls, timedCall{step: step, result: result})
	}
	slices.SortFunc(calls, func(a, b timedCall) int { return a.step - b.step })
	slices.Sort(untimed)
	return calls, untimed
}
