package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The two keys COMMON.md freezes. The step number is how the gateway ties a
// call to the decision it wrote about it, and it is the only place that mapping
// exists: the runner reads it back out of the evidence trail, never out of the
// log this file writes.
const (
	metaStep  = "io.guardana.playground/step"
	metaRunID = "io.guardana.playground/run-id"
)

// toolCaller is the one thing this agent does to the gateway. It is an
// interface so a test can put a denial or a dropped connection at any step
// without a gateway to run against; *mcp.ClientSession satisfies it.
type toolCaller interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
}

// outcome is what happened to one step. A denial is not a failure of the run:
// it is the answer a working policy gives, and the trajectory carries on so
// that a later step depending on denied data fails where the data is missing.
type outcome string

const (
	served outcome = "served"
	denied outcome = "denied"
	failed outcome = "error"
)

// replay sends every step of the trajectory in order and returns the first
// reason a step could not be sent at all.
func replay(
	ctx context.Context, caller toolCaller, trajectory labspec.Trajectory, runID, namespace string, log io.Writer,
	tracer *traceWriter,
) error {
	outputs := make(map[int]string, len(trajectory.Steps))
	journal := &stepLog{to: log, runID: runID, namespace: namespace}

	for i, step := range trajectory.Steps {
		number := i + 1
		if err := pause(ctx, time.Duration(step.WaitBefore)); err != nil {
			journal.record(number, step, failed, 0, err)
			return fmt.Errorf("step %d: waiting before the call: %w", number, err)
		}
		arguments, err := step.Resolve(outputs)
		if err != nil {
			journal.record(number, step, failed, 0, err)
			return fmt.Errorf("step %d: %w", number, err)
		}
		result, pended, err := callWhilePending(ctx, caller, number, step, &mcp.CallToolParams{
			Name:      step.Call.Tool,
			Arguments: arguments,
			Meta:      mcp.Meta{metaStep: number, metaRunID: runID},
		}, journal)
		if err != nil {
			journal.record(number, step, failed, 0, err)
			return fmt.Errorf("step %d: %s on %s: %w", number, step.Call.Tool, step.Call.Server, err)
		}
		// A denied call is recorded as an empty output rather than left out, so
		// a step reading it is refused for reading nothing rather than sending
		// the reference through unresolved.
		output := ""
		if !result.IsError {
			output = textOf(result)
		}
		outputs[number] = output
		journal.record(number, step, statusOf(result, namespace), len(output), nil)
		if tracer != nil {
			if err := tracer.record(number, step, arguments, result, pended); err != nil {
				return fmt.Errorf("step %d: tracing: %w", number, err)
			}
		}
	}
	return journal.err
}

func statusOf(result *mcp.CallToolResult, namespace string) outcome {
	if isPending(result, namespace) {
		return pendingOutcome
	}
	if result.IsError {
		return denied
	}
	return served
}

// textOf is the step's output: the text content of the result, in order, with
// no separator. Anything that is not text is not part of the payload a flow
// carries and is left out.
func textOf(result *mcp.CallToolResult) string {
	var text strings.Builder
	for _, content := range result.Content {
		if item, ok := content.(*mcp.TextContent); ok {
			text.WriteString(item.Text)
		}
	}
	return text.String()
}

// stepLog writes one JSON object per attempted step.
//
// Nothing asserts on this file. It records how much a step produced and never
// what it produced, so a canary planted in a victim's fixtures does not end up
// in a file the lab keeps.
type stepLog struct {
	to        io.Writer
	runID     string
	namespace string
	err       error
}

type logLine struct {
	At          time.Time `json:"at"`
	RunID       string    `json:"run_id"`
	Step        int       `json:"step"`
	Label       string    `json:"label,omitempty"`
	Server      string    `json:"server"`
	Tool        string    `json:"tool"`
	Outcome     outcome   `json:"outcome"`
	OutputBytes int       `json:"output_bytes"`
	Error       string    `json:"error,omitempty"`
}

func (l *stepLog) record(number int, step labspec.Step, result outcome, size int, cause error) {
	if l.err != nil {
		return
	}
	line := logLine{
		At:          time.Now().UTC(),
		RunID:       l.runID,
		Step:        number,
		Label:       step.Label,
		Server:      step.Call.Server,
		Tool:        step.Call.Tool,
		Outcome:     result,
		OutputBytes: size,
	}
	if cause != nil {
		line.Error = cause.Error()
	}
	encoded, err := json.Marshal(line)
	if err != nil {
		l.err = err
		return
	}
	_, l.err = l.to.Write(append(encoded, '\n'))
}
