package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// testNamespace stands in for the enforcer's, which the runner reads from
// versions.env and hands the agent as a flag.
const testNamespace = "lab.test"

// pendingAnswer is shaped as the enforcer's gateway answers a held call: an
// error result with the reason code, marked under the gateway's namespace.
func pendingAnswer() answer {
	return answer{
		isError:    true,
		text:       []string{"APPROVAL_PENDING"},
		structured: map[string]any{"reason_code": "APPROVAL_PENDING", "approval_id": "a-1", "retry_after": float64(1)},
		meta:       mcp.Meta{testNamespace + "/answer": "pending"},
	}
}

func heldStep(atMost int) labspec.Trajectory {
	trajectory := twoSteps()
	trajectory.Steps = trajectory.Steps[:1]
	trajectory.Steps[0].RetryWhilePending = &labspec.Retry{Every: labspec.Duration(time.Millisecond), AtMost: atMost}
	return trajectory
}

func TestAHeldCallIsSentAgainUntilItIsNoLongerPending(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{pendingAnswer(), pendingAnswer(), {text: []string{"id,name"}}}}
	var log bytes.Buffer
	if err := replay(context.Background(), gateway, heldStep(5), "run-1", testNamespace, &log); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 3 {
		t.Fatalf("sent %d calls, want 3: two pending answers, then the one that ran", len(gateway.sent))
	}
	if strings.Count(log.String(), `"outcome":"pending"`) != 2 || !strings.Contains(log.String(), `"outcome":"served"`) {
		t.Errorf("the log does not show two pending answers and one served:\n%s", log.String())
	}
}

func TestRetriesStopAtTheirBound(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{pendingAnswer(), pendingAnswer(), pendingAnswer(), pendingAnswer()}}
	var log bytes.Buffer
	if err := replay(context.Background(), gateway, heldStep(2), "run-1", testNamespace, &log); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 3 {
		t.Errorf("sent %d calls, want the call and two retries", len(gateway.sent))
	}
	if strings.Count(log.String(), `"outcome":"pending"`) != 3 {
		t.Errorf("the log does not show the step still pending when the retries ran out:\n%s", log.String())
	}
}

// Only the gateway's own mark makes an answer pending: an upstream result can
// carry the same reason code, and the gateway strips its namespace from them.
func TestAnAnswerIsPendingOnlyUnderTheGatewaysMark(t *testing.T) {
	for name, change := range map[string]func(*answer){
		"no mark":                  func(a *answer) { a.meta = nil },
		"another namespace's mark": func(a *answer) { a.meta = mcp.Meta{"upstream.example/answer": "pending"} },
		"the mark of a block":      func(a *answer) { a.meta = mcp.Meta{testNamespace + "/answer": "blocked"} },
		"not an error":             func(a *answer) { a.isError = false },
		"another reason code":      func(a *answer) { a.structured = map[string]any{"reason_code": "RULE_DENY"} },
		"no structured content":    func(a *answer) { a.structured = nil },
	} {
		t.Run(name, func(t *testing.T) {
			shaped := pendingAnswer()
			change(&shaped)
			gateway := &fakeGateway{answers: []answer{shaped, {text: []string{"ok"}}}}
			var log bytes.Buffer
			if err := replay(context.Background(), gateway, heldStep(5), "run-1", testNamespace, &log); err != nil {
				t.Fatalf("replay: %v", err)
			}
			if len(gateway.sent) != 1 || strings.Contains(log.String(), `"outcome":"pending"`) {
				t.Errorf("sent %d calls and logged:\n%s", len(gateway.sent), log.String())
			}
		})
	}
}

func TestADenialIsNotRetriedAndNeitherIsAStepThatAsksForNoRetry(t *testing.T) {
	denied := answer{isError: true, text: []string{"RULE_DENY"}, structured: map[string]any{"reason_codes": []any{"RULE_DENY"}}}
	gateway := &fakeGateway{answers: []answer{denied}}
	if err := replay(context.Background(), gateway, heldStep(5), "run-1", testNamespace, &bytes.Buffer{}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 1 {
		t.Errorf("a denial was sent %d times, want once", len(gateway.sent))
	}
	noRetry := heldStep(5)
	noRetry.Steps[0].RetryWhilePending = nil
	gateway = &fakeGateway{answers: []answer{pendingAnswer()}}
	if err := replay(context.Background(), gateway, noRetry, "run-1", testNamespace, &bytes.Buffer{}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 1 {
		t.Errorf("a pending answer on a step with no retry was sent %d times, want once", len(gateway.sent))
	}
}

func TestAStepWaitsBeforeItsCall(t *testing.T) {
	trajectory := heldStep(1)
	trajectory.Steps[0].RetryWhilePending = nil
	trajectory.Steps[0].WaitBefore = labspec.Duration(300 * time.Millisecond)
	gateway := &fakeGateway{answers: []answer{{text: []string{"ok"}}}}
	started := time.Now()
	if err := replay(context.Background(), gateway, trajectory, "run-1", testNamespace, &bytes.Buffer{}); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if elapsed := time.Since(started); elapsed < 300*time.Millisecond {
		t.Errorf("the call went after %s, want at least 300ms", elapsed)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := replay(ctx, &fakeGateway{answers: []answer{{text: []string{"ok"}}}}, trajectory, "run-1", testNamespace, &bytes.Buffer{}); err == nil {
		t.Error("a wait under a cancelled context returned no error")
	}
}

func TestTheDeadlineLeavesRoomForTheWaits(t *testing.T) {
	trajectory := heldStep(4)
	trajectory.Steps[0].WaitBefore = labspec.Duration(time.Second)
	trajectory.Steps[0].RetryWhilePending.Every = labspec.Duration(2 * time.Second)
	if got := replayDeadline(2*time.Minute, trajectory); got != 2*time.Minute+9*time.Second {
		t.Errorf("deadline = %s, want 2m9s", got)
	}
}
