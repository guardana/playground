package main

import (
	"context"
	"time"

	gateanswer "github.com/guardana/playground/drivers/control/answer"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const pendingOutcome outcome = "pending"

// callWhilePending sends the call, and sends it again while the step allows and
// the gateway answers that the request is still held. Every pending answer is
// logged, so a person reading a red run sees how long the agent waited.
func callWhilePending(
	ctx context.Context, caller toolCaller, number int, step labspec.Step, params *mcp.CallToolParams, log *stepLog,
) (*mcp.CallToolResult, bool, error) {
	pended := false
	for retries := 0; ; retries++ {
		result, err := caller.CallTool(ctx, params)
		held := err == nil && gateanswer.Classify(result, log.namespace) == record.AnswerHeld
		pended = pended || held
		if err != nil || step.RetryWhilePending == nil || !held ||
			retries >= step.RetryWhilePending.AtMost {
			return result, pended, err
		}
		log.record(number, step, pendingOutcome, 0, nil)
		if err := pause(ctx, time.Duration(step.RetryWhilePending.Every)); err != nil {
			return nil, pended, err
		}
	}
}

func pause(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// replayDeadline is the replay's own bound plus the time its steps wait on
// purpose, so a trajectory that waits for an approval is not cut short.
func replayDeadline(base time.Duration, trajectory labspec.Trajectory) time.Duration {
	return base + budget(trajectory)
}

// budget is the time the trajectory spends waiting on purpose.
func budget(trajectory labspec.Trajectory) time.Duration {
	var total time.Duration
	for _, step := range trajectory.Steps {
		total += step.Budget()
	}
	return total
}
