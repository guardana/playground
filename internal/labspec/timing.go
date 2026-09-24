package labspec

import (
	"encoding/json"
	"fmt"
	"time"
)

// Bounds on the time a trajectory may spend not calling anything. A step that
// waits is a step whose deadline the agent has to extend, and an unbounded one
// is a run that never reports.
const (
	MaxWait       = 10 * time.Minute
	MinRetryEvery = 100 * time.Millisecond
	MaxRetryEvery = time.Minute
	MaxRetries    = 100
)

// Duration is a Go duration written as a string, such as "1500ms" or "2m".
type Duration time.Duration

// UnmarshalJSON accepts only the string form: a bare number would be read as
// nanoseconds, which nobody writing a scenario means.
func (d *Duration) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("%w: a duration is written as a string such as \"2s\", got %s", ErrInvalid, raw)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	*d = Duration(parsed)
	return nil
}

// Retry resends a step's call while the gateway answers that the request is
// held and pending, every Every, at most AtMost more times. A pending answer
// writes nothing to the trail; the call that is finally answered is the one
// the scenario grades.
type Retry struct {
	Every  Duration `json:"every"`
	AtMost int      `json:"at_most"`
}

// Budget is the longest the step can take besides its calls.
func (s Step) Budget() time.Duration {
	budget := time.Duration(s.WaitBefore)
	if s.RetryWhilePending != nil {
		budget += time.Duration(s.RetryWhilePending.Every) * time.Duration(s.RetryWhilePending.AtMost)
	}
	return budget
}

func (s Step) validateTiming(number int) error {
	wait := time.Duration(s.WaitBefore)
	if wait < 0 || wait > MaxWait {
		return fmt.Errorf("%w: steps[%d].wait_before is %s, want 0 to %s", ErrInvalid, number, wait, MaxWait)
	}
	retry := s.RetryWhilePending
	if retry == nil {
		return nil
	}
	every := time.Duration(retry.Every)
	switch {
	case every < MinRetryEvery || every > MaxRetryEvery:
		return fmt.Errorf("%w: steps[%d].retry_while_pending.every is %s, want %s to %s",
			ErrInvalid, number, every, MinRetryEvery, MaxRetryEvery)
	case retry.AtMost < 1 || retry.AtMost > MaxRetries:
		return fmt.Errorf("%w: steps[%d].retry_while_pending.at_most is %d, want 1 to %d",
			ErrInvalid, number, retry.AtMost, MaxRetries)
	case every*time.Duration(retry.AtMost) > MaxWait:
		return fmt.Errorf("%w: steps[%d] retries for %s, longer than %s",
			ErrInvalid, number, every*time.Duration(retry.AtMost), MaxWait)
	}
	return nil
}
