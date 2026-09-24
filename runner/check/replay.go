package check

import (
	"context"
	"fmt"

	"github.com/guardana/playground/internal/assertion"
)

// Replay reports whether the scripted agent sent the trajectory.
//
// The outcome is read from the exit status the runner observed, never from the
// agent's own log: an agent's account of its own work is not evidence, and a
// scenario is graded from the trail and the journals. This check exists because
// a run in which no call was ever sent must not be able to reach the other
// checks and be graded on a silence.
type Replay struct {
	// Ran reports that the agent was started at all. A run that never started
	// establishes nothing, which is not the same as a run that failed.
	Ran      bool
	ExitCode int
	Detail   string
	// Source is where the runner wrote what the agent printed.
	Source string
}

// ID names the check in a report.
func (Replay) ID() string { return "trajectory" }

// Run grades the replay.
func (r Replay) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	result := assertion.Result{
		Check:  "trajectory/replayed",
		Want:   "every step sent",
		Source: r.Source,
		Detail: spoken(r.Detail),
	}
	switch {
	case !r.Ran:
		result.Outcome = assertion.Indeterminate
		result.Got = "the agent was not run"
	case r.ExitCode == 0:
		result.Outcome = assertion.Pass
		result.Got = "every step sent"
	default:
		result.Outcome = assertion.Fail
		result.Got = fmt.Sprintf("the agent exited %d, so a step could not be sent", r.ExitCode)
	}
	return []assertion.Result{result}, nil
}
