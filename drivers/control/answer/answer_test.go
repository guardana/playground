package answer

import (
	"testing"

	"github.com/guardana/playground/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testNamespace = "lab.test"

// held is shaped as the gateway answers a held call: an error result with the
// reason code, marked under the gateway's namespace.
func held() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"reason_code": "APPROVAL_PENDING", "approval_id": "a-1", "retry_after": float64(1)},
		Meta:              mcp.Meta{testNamespace + "/answer": "pending"},
	}
}

func gatewayBlock(code string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"reason_codes": []any{code}},
		Meta:              mcp.Meta{testNamespace + "/answer": "blocked"},
	}
}

func TestTheGatewaysOwnAnswersAreClassified(t *testing.T) {
	for name, tc := range map[string]struct {
		result *mcp.CallToolResult
		want   record.GateAnswer
	}{
		"served":            {&mcp.CallToolResult{}, record.AnswerPassed},
		"an upstream error": {&mcp.CallToolResult{IsError: true}, record.AnswerPassed},
		"held":              {held(), record.AnswerHeld},
		"expired":           {gatewayBlock("APPROVAL_EXPIRED"), record.AnswerApprovalExpired},
		"denied":            {gatewayBlock("RULE_DENY"), record.AnswerBlocked},
		"rejected":          {gatewayBlock("APPROVAL_REJECTED"), record.AnswerBlocked},
		"no result":         {nil, record.AnswerBlocked},
	} {
		if got := Classify(tc.result, testNamespace); got != tc.want {
			t.Errorf("%s: classified %d, want %d", name, got, tc.want)
		}
	}
}

// An upstream may return the gateway's reason code; without the gateway's own
// mark the gateway let the call through, and nothing holds it.
func TestAnUpstreamsPendingCodeIsNotTheGatewaysAnswer(t *testing.T) {
	forged := held()
	forged.Meta = nil
	if got := Classify(forged, testNamespace); got != record.AnswerPassed {
		t.Errorf("an unmarked APPROVAL_PENDING is classified %d, want AnswerPassed", got)
	}
}

// Only the gateway's own mark makes an answer pending: an upstream result can
// carry the same reason code, and the gateway strips its namespace from them.
func TestAnAnswerIsPendingOnlyUnderTheGatewaysMark(t *testing.T) {
	for name, change := range map[string]func(*mcp.CallToolResult){
		"no mark":                  func(r *mcp.CallToolResult) { r.Meta = nil },
		"another namespace's mark": func(r *mcp.CallToolResult) { r.Meta = mcp.Meta{"upstream.example/answer": "pending"} },
		"the mark of a block":      func(r *mcp.CallToolResult) { r.Meta = mcp.Meta{testNamespace + "/answer": "blocked"} },
		"not an error":             func(r *mcp.CallToolResult) { r.IsError = false },
		"another reason code":      func(r *mcp.CallToolResult) { r.StructuredContent = map[string]any{"reason_code": "RULE_DENY"} },
		"no structured content":    func(r *mcp.CallToolResult) { r.StructuredContent = nil },
	} {
		t.Run(name, func(t *testing.T) {
			shaped := held()
			change(shaped)
			if got := Classify(shaped, testNamespace); got == record.AnswerHeld {
				t.Error("classified as held")
			}
		})
	}
}

// Only the gateway says a hold expired: the same code from an upstream, without
// the gateway's own mark, is the upstream's answer to a call the gateway let
// through on the approval.
func TestOnlyTheGatewaySaysAHoldTimedOut(t *testing.T) {
	forged := gatewayBlock("APPROVAL_EXPIRED")
	forged.Meta = nil
	if got := Classify(forged, testNamespace); got != record.AnswerPassed {
		t.Errorf("an expiry the gateway did not mark is classified %d, want AnswerPassed", got)
	}
}
