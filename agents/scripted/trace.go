package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// traceVersion is the verifier's native trace dialect this agent writes.
const traceVersion = 3

// approvalExpired is the reason code the gateway blocks a held call with once
// its approval expired, and blockedMark the value it puts under
// `<namespace>/answer` on a block it made itself.
const (
	approvalExpired = "APPROVAL_EXPIRED"
	blockedMark     = "blocked"
)

// traceWriter records what this agent saw, as a team's own instrumentation
// would: one tool span per step, the effect the call had as the gateway
// answered it, and whether an approval was sought for it. It records approvals
// and effects, so the verifier grades both; it cannot know who approved, so it
// never says.
type traceWriter struct {
	runID     string
	namespace string
	spans     []traceSpan
	// held holds every call the gateway still held when its step ended, so a
	// later step sending the same call again resumes an approval that was sought.
	held map[string]bool
}

type traceSpan struct {
	SpanID    string          `json:"span_id"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Tool      traceTool       `json:"tool"`
	Effects   []traceEffect   `json:"effects"`
	Approvals []traceApproval `json:"approvals"`
}

type traceTool struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Status    string `json:"status"`
}

type traceEffect struct {
	Sink   string `json:"sink"`
	Action string `json:"action"`
	Target string `json:"target"`
	Status string `json:"status"`
}

type traceApproval struct {
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

// record adds one step: result is the final answer, pended whether any answer
// before it was the pending one.
func (w *traceWriter) record(number int, step labspec.Step, arguments map[string]any, result *mcp.CallToolResult, pended bool) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	call := step.Call.Server + "\x00" + step.Call.Tool + "\x00" + string(encoded)
	stillPending := isPending(result, w.namespace)
	sought := pended || w.held[call]
	// A hold is spent by the step that ends it, run or refused: the same call
	// sent later without a new hold rides on no approval.
	if stillPending {
		if w.held == nil {
			w.held = map[string]bool{}
		}
		w.held[call] = true
	} else {
		delete(w.held, call)
	}
	ran := result != nil && !result.IsError
	// The gateway marks every answer it makes itself and strips the mark from
	// an upstream's, so an unmarked error is the upstream's answer to a call
	// the gateway let through: the effect may have happened.
	passed := ran || (result != nil && result.Meta[w.namespace+"/answer"] == nil)
	span := traceSpan{
		SpanID: fmt.Sprintf("s%d", number), Kind: "tool_execution", Name: step.Call.Tool,
		Tool: traceTool{Name: step.Call.Tool, Arguments: string(encoded), Status: "failed"},
	}
	effect := traceEffect{Sink: sinkOf(step.Call.Server), Action: step.Call.Tool, Target: step.Call.Server, Status: "failed"}
	switch {
	case ran:
		span.Tool.Status, effect.Status = "succeeded", "executed"
	case passed:
		effect.Status = "attempted"
	}
	span.Effects = []traceEffect{effect}
	span.Approvals = []traceApproval{{Action: step.Call.Tool, Outcome: w.approvalOutcome(result, stillPending, passed, sought)}}
	w.spans = append(w.spans, span)
	return nil
}

// approvalOutcome speaks the verifier's approval words: unknown while the call
// is still held, and for a held call that ended, granted when the gateway let
// it through, timed_out when the gateway refused it because its approval
// expired, denied for any other refusal.
func (w *traceWriter) approvalOutcome(result *mcp.CallToolResult, stillPending, passed, sought bool) string {
	switch {
	case stillPending:
		return "unknown"
	case !sought:
		return "not_requested"
	case passed:
		return "granted"
	case gatewayRefused(result, w.namespace, approvalExpired):
		return "timed_out"
	default:
		return "denied"
	}
}

// gatewayRefused reports whether the gateway itself blocked the call with this
// reason code. Only the gateway writes under its namespace, so an upstream
// result carrying the same code is not read as the gateway's.
func gatewayRefused(result *mcp.CallToolResult, namespace, code string) bool {
	if result == nil || !result.IsError || result.Meta[namespace+"/answer"] != blockedMark {
		return false
	}
	fields, ok := result.StructuredContent.(map[string]any)
	if !ok {
		return false
	}
	codes, _ := fields["reason_codes"].([]any)
	return slices.Contains(codes, any(code))
}

// sinkOf names the effect sink a victim's calls land on, from the verifier's
// closed list.
func sinkOf(server string) string {
	switch server {
	case "victim-db":
		return "sql"
	case "victim-shell":
		return "shell"
	case "victim-fs":
		return "filesystem"
	case "victim-web":
		return "http"
	case "victim-mail":
		return "email"
	default:
		return "other"
	}
}

// write emits the header, every span and, for a replay that ran every step, the
// footer that says the file ended where the producer meant it to; without it
// the verifier reads the trace as cut short.
func (w *traceWriter) write(out io.Writer, complete bool) error {
	encoder := json.NewEncoder(out)
	header := map[string]any{
		"guardana_trace": traceVersion, "trace_id": w.runID, "terminated": true,
		"producer":     map[string]string{"name": agentName, "version": agentVersion},
		"instrumented": []string{"tools", "approval", "effects"},
	}
	if err := encoder.Encode(header); err != nil {
		return err
	}
	for _, span := range w.spans {
		if err := encoder.Encode(span); err != nil {
			return err
		}
	}
	if !complete {
		return nil
	}
	return encoder.Encode(map[string]int{"guardana_trace_end": traceVersion, "spans": len(w.spans)})
}
