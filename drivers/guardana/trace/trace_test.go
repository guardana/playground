package trace

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func traceStep(server, tool string) labspec.Step {
	return labspec.Step{Call: labspec.Call{Server: server, Tool: tool}}
}

// answered is one step's final result and what the gate made of it.
type answered struct {
	result *mcp.CallToolResult
	gate   record.GateAnswer
}

func served() answered { return answered{&mcp.CallToolResult{}, record.AnswerPassed} }

// failing is an error result the gate read as the given answer.
func failing(gate record.GateAnswer) answered {
	return answered{&mcp.CallToolResult{IsError: true}, gate}
}

func TestTheTraceRecordsEachCallsEffectAndApproval(t *testing.T) {
	tracer := New("run-1", Producer{})
	blocked := failing(record.AnswerBlocked)
	refusedUpstream := failing(record.AnswerPassed)
	for number, call := range []struct {
		step   labspec.Step
		answer answered
		pended bool
	}{
		{traceStep("victim-crm", "crm.read_customer"), served(), false},
		{traceStep("victim-shell", "shell.exec"), blocked, false},
		{traceStep("victim-crm", "crm.update_bank_account"), served(), true},
		{traceStep("victim-crm", "crm.update_bank_account"), failing(record.AnswerHeld), true},
		{traceStep("victim-mail", "mail.send"), blocked, true},
		{traceStep("victim-shell", "shell.exec"), refusedUpstream, false},
		{traceStep("victim-mail", "mail.send"), refusedUpstream, true},
	} {
		if err := tracer.Record(number+1, call.step, map[string]any{"k": "v"}, call.answer.result, call.answer.gate, call.pended); err != nil {
			t.Fatal(err)
		}
	}
	want := []struct{ sink, effect, approval string }{
		{"other", "executed", "not_requested"},
		{"shell", "failed", "not_requested"},
		{"other", "executed", "granted"},
		{"other", "failed", "unknown"},
		{"email", "failed", "denied"},
		{"shell", "attempted", "not_requested"},
		{"email", "attempted", "granted"},
	}
	for i, span := range tracer.spans {
		got := [3]string{span.Effects[0].Sink, span.Effects[0].Status, span.Approvals[0].Outcome}
		if got != [3]string{want[i].sink, want[i].effect, want[i].approval} {
			t.Errorf("span %d = %v, want %+v", i+1, got, want[i])
		}
	}
}

// A call held in one step and sent again in a later one resumes the approval
// the first sought; a different call is a request of its own.
func TestACallResumedInALaterStepWasApproved(t *testing.T) {
	tracer := New("run-1", Producer{})
	change := traceStep("victim-crm", "crm.update_bank_account")
	for number, call := range []struct {
		arguments map[string]any
		answer    answered
	}{
		{map[string]any{"account": "9001"}, failing(record.AnswerHeld)},
		{map[string]any{"account": "9001"}, served()},
		{map[string]any{"account": "6666"}, served()},
	} {
		if err := tracer.Record(number+1, change, call.arguments, call.answer.result, call.answer.gate, false); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"unknown", "granted", "not_requested"} {
		if got := tracer.spans[i].Approvals[0].Outcome; got != want {
			t.Errorf("span %d approval = %s, want %s", i+1, got, want)
		}
	}
}

// outcomesOf records the same call once per answer, each in a step of its own.
func outcomesOf(t *testing.T, answers ...answered) []string {
	t.Helper()
	tracer := New("run-1", Producer{})
	change := traceStep("victim-crm", "crm.update_bank_account")
	for number, answer := range answers {
		if err := tracer.Record(number+1, change, map[string]any{"account": "9001"}, answer.result, answer.gate, false); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, span := range tracer.spans {
		got = append(got, span.Approvals[0].Outcome)
	}
	return got
}

// A hold ends with the step that ends it: a call that runs later with no new
// hold ran on no approval the trace can name.
func TestAHoldIsSpentByTheStepThatEndsIt(t *testing.T) {
	held, rejected := failing(record.AnswerHeld), failing(record.AnswerBlocked)
	for name, tc := range map[string]struct {
		answers []answered
		want    []string
	}{
		"rejected, then run": {
			[]answered{held, rejected, served()},
			[]string{"unknown", "denied", "not_requested"},
		},
		"approved and run, then run again": {
			[]answered{held, served(), served()},
			[]string{"unknown", "granted", "not_requested"},
		},
		"expired, then run": {
			[]answered{held, failing(record.AnswerApprovalExpired), served()},
			[]string{"unknown", "timed_out", "not_requested"},
		},
		"held again after a refusal": {
			[]answered{held, rejected, held, served()},
			[]string{"unknown", "denied", "unknown", "granted"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := outcomesOf(t, tc.answers...); !slices.Equal(got, tc.want) {
				t.Errorf("outcomes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTheTraceSaysWhetherItEndedWhereTheProducerMeantItTo(t *testing.T) {
	tracer := New("run-1", Producer{})
	if err := tracer.Record(1, traceStep("victim-fs", "fs.read"), nil, &mcp.CallToolResult{}, record.AnswerPassed, false); err != nil {
		t.Fatal(err)
	}
	var whole, cut bytes.Buffer
	if err := tracer.Write(&whole, true); err != nil {
		t.Fatal(err)
	}
	if err := tracer.Write(&cut, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(whole.String()), "\n")
	if len(lines) != 3 || lines[2] != `{"guardana_trace_end":3,"spans":1}` {
		t.Errorf("a complete trace ends %q", lines[len(lines)-1])
	}
	var header map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatal(err)
	}
	if header["guardana_trace"] != float64(3) || header["terminated"] != true || header["trace_id"] != "run-1" {
		t.Errorf("header = %v", header)
	}
	if strings.Contains(cut.String(), "guardana_trace_end") {
		t.Error("a trace cut short carries the footer that says it ended")
	}
}
