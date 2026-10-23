// Package trace writes an agent's own account of a run in the verifier's
// native trace dialect, so the verifier can grade it.
package trace

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// traceVersion is the verifier's native trace dialect this package writes.
const traceVersion = 3

// Producer names the program whose account the trace is.
type Producer struct {
	Name    string
	Version string
}

// Writer records what an agent saw, as a team's own instrumentation would:
// one tool span per step, the effect the call had as the gate answered it,
// and whether an approval was sought for it. It records approvals and effects,
// so the verifier grades both; it cannot know who approved, so it never says.
type Writer struct {
	runID    string
	producer Producer
	spans    []traceSpan
	// held holds every call the gate still held when its step ended, so a
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

// New returns a Writer for one run.
func New(runID string, producer Producer) *Writer {
	return &Writer{runID: runID, producer: producer}
}

// Record adds one step: result is the final answer, gate what the gate made of
// it, and pended whether any answer before it was the gate holding the call.
func (w *Writer) Record(
	number int, step labspec.Step, arguments map[string]any, result *mcp.CallToolResult, gate record.GateAnswer, pended bool,
) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	call := step.Call.Server + "\x00" + step.Call.Tool + "\x00" + string(encoded)
	stillPending := gate == record.AnswerHeld
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
	// An error the gate let through is the upstream's answer: the effect may
	// have happened.
	passed := ran || gate == record.AnswerPassed
	span := traceSpan{
		SpanID: fmt.Sprintf("s%d", number), Kind: "tool_execution", Name: step.Call.Tool,
		Tool: traceTool{Name: step.Call.Tool, Arguments: string(encoded), Status: "failed"},
	}
	sink, err := sinkOf(step.Call.Server)
	if err != nil {
		return err
	}
	effect := traceEffect{Sink: sink, Action: step.Call.Tool, Target: step.Call.Server, Status: "failed"}
	switch {
	case ran:
		span.Tool.Status, effect.Status = "succeeded", "executed"
	case passed:
		effect.Status = "attempted"
	}
	span.Effects = []traceEffect{effect}
	span.Approvals = []traceApproval{{Action: step.Call.Tool, Outcome: approvalOutcome(gate, stillPending, passed, sought)}}
	w.spans = append(w.spans, span)
	return nil
}

// approvalOutcome speaks the verifier's approval words: unknown while the call
// is still held, and for a held call that ended, granted when the gate let it
// through, timed_out when the gate refused it because its approval expired,
// denied for any other refusal.
func approvalOutcome(gate record.GateAnswer, stillPending, passed, sought bool) string {
	switch {
	case stillPending:
		return "unknown"
	case !sought:
		return "not_requested"
	case passed:
		return "granted"
	case gate == record.AnswerApprovalExpired:
		return "timed_out"
	default:
		return "denied"
	}
}

// sinks names the effect sink each victim's calls land on, from the
// verifier's closed list.
func sinks() map[string]string {
	return map[string]string{
		"victim-crm": "other", "victim-db": "sql", "victim-fs": "filesystem",
		"victim-shell": "shell", "victim-mail": "email", "victim-web": "http",
		"victim-pay": "payment",
	}
}

// sinkOf refuses a server it has no sink for rather than call it "other": a
// contract forbidding that server's sink would then never match its calls.
func sinkOf(server string) (string, error) {
	sink, known := sinks()[server]
	if !known {
		return "", fmt.Errorf("trace: no effect sink is known for %s", server)
	}
	return sink, nil
}

// Write emits the header, every span and, for a replay that ran every step, the
// footer that says the file ended where the producer meant it to; without it
// the verifier reads the trace as cut short.
func (w *Writer) Write(out io.Writer, complete bool) error {
	encoder := json.NewEncoder(out)
	header := map[string]any{
		"guardana_trace": traceVersion, "trace_id": w.runID, "terminated": true,
		"producer":     map[string]string{"name": w.producer.Name, "version": w.producer.Version},
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
