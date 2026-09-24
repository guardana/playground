package main

import (
	"context"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// approvalPending is the reason code the enforcer's gateway answers a held
// request with, in the result's structured content, and pendingMark the value
// it puts under `<namespace>/answer` in `_meta` to say it made that answer
// itself. Both are read only to decide whether to send the call again, never
// to grade anything.
const (
	approvalPending = "APPROVAL_PENDING"
	pendingMark     = "pending"
)

const pendingOutcome outcome = "pending"

// isPending reports whether the gateway itself answered that the request is
// held. The reason code alone is not enough: an upstream tool can return it,
// and only the gateway may write under its own namespace.
func isPending(result *mcp.CallToolResult, namespace string) bool {
	if result == nil || !result.IsError || result.Meta[namespace+"/answer"] != pendingMark {
		return false
	}
	fields, ok := result.StructuredContent.(map[string]any)
	return ok && fields["reason_code"] == approvalPending
}

// callWhilePending sends the call, and sends it again while the step allows and
// the gateway answers that the request is still held. Every pending answer is
// logged, so a person reading a red run sees how long the agent waited.
func callWhilePending(
	ctx context.Context, caller toolCaller, number int, step labspec.Step, params *mcp.CallToolParams, log *stepLog,
) (*mcp.CallToolResult, bool, error) {
	pended := false
	for retries := 0; ; retries++ {
		result, err := caller.CallTool(ctx, params)
		pended = pended || (err == nil && isPending(result, log.namespace))
		if err != nil || step.RetryWhilePending == nil || !isPending(result, log.namespace) ||
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
