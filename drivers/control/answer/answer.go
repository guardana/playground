// Package answer reads what the enforcer's gateway answered one call, and is
// the only place that knows the marks and reason codes it answers with.
package answer

import (
	"slices"

	"github.com/guardana/playground/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The gateway marks every answer it makes itself with one of these values
// under `<namespace>/answer` in `_meta`, and strips that key from an
// upstream's result.
const (
	pendingMark = "pending"
	blockedMark = "blocked"
)

// approvalPending is the reason code of a held call, in `reason_code`;
// approvalExpired the one of a call refused because its approval expired, in
// `reason_codes`.
const (
	approvalPending = "APPROVAL_PENDING"
	approvalExpired = "APPROVAL_EXPIRED"
)

// Classify reads one result as the gateway's answer. Only the gateway writes
// under its namespace, so a reason code alone, which an upstream can return
// too, never makes an answer the gateway's.
func Classify(result *mcp.CallToolResult, namespace string) record.GateAnswer {
	if result == nil {
		return record.AnswerBlocked
	}
	if !result.IsError {
		return record.AnswerPassed
	}
	mark := result.Meta[namespace+"/answer"]
	fields, _ := result.StructuredContent.(map[string]any)
	switch {
	case mark == nil:
		return record.AnswerPassed
	case mark == pendingMark && fields["reason_code"] == approvalPending:
		return record.AnswerHeld
	case mark == blockedMark && hasCode(fields, approvalExpired):
		return record.AnswerApprovalExpired
	default:
		return record.AnswerBlocked
	}
}

func hasCode(fields map[string]any, code string) bool {
	codes, _ := fields["reason_codes"].([]any)
	return slices.Contains(codes, any(code))
}
