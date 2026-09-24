package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func traceStep(server, tool string) labspec.Step {
	return labspec.Step{Call: labspec.Call{Server: server, Tool: tool}}
}

func pendingResult() *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"reason_code": "APPROVAL_PENDING"},
		Meta:              mcp.Meta{testNamespace + "/answer": "pending"},
	}
}

func TestTheTraceRecordsEachCallsEffectAndApproval(t *testing.T) {
	tracer := &traceWriter{runID: "run-1", namespace: testNamespace}
	served := &mcp.CallToolResult{}
	blocked := gatewayBlock("RULE_DENY")
	refusedUpstream := &mcp.CallToolResult{IsError: true}
	for number, call := range []struct {
		step   labspec.Step
		result *mcp.CallToolResult
		pended bool
	}{
		{traceStep("victim-crm", "crm.read_customer"), served, false},
		{traceStep("victim-shell", "shell.exec"), blocked, false},
		{traceStep("victim-crm", "crm.update_bank_account"), served, true},
		{traceStep("victim-crm", "crm.update_bank_account"), pendingResult(), true},
		{traceStep("victim-mail", "mail.send"), blocked, true},
		{traceStep("victim-shell", "shell.exec"), refusedUpstream, false},
		{traceStep("victim-mail", "mail.send"), refusedUpstream, true},
	} {
		if err := tracer.record(number+1, call.step, map[string]any{"k": "v"}, call.result, call.pended); err != nil {
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
	tracer := &traceWriter{runID: "run-1", namespace: testNamespace}
	change := traceStep("victim-crm", "crm.update_bank_account")
	for number, call := range []struct {
		arguments map[string]any
		result    *mcp.CallToolResult
	}{
		{map[string]any{"account": "9001"}, pendingResult()},
		{map[string]any{"account": "9001"}, &mcp.CallToolResult{}},
		{map[string]any{"account": "6666"}, &mcp.CallToolResult{}},
	} {
		if err := tracer.record(number+1, change, call.arguments, call.result, false); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range []string{"unknown", "granted", "not_requested"} {
		if got := tracer.spans[i].Approvals[0].Outcome; got != want {
			t.Errorf("span %d approval = %s, want %s", i+1, got, want)
		}
	}
}

func gatewayBlock(code string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError:           true,
		StructuredContent: map[string]any{"reason_codes": []any{code}},
		Meta:              mcp.Meta{testNamespace + "/answer": "blocked"},
	}
}

// outcomesOf records the same call once per result, each in a step of its own.
func outcomesOf(t *testing.T, results ...*mcp.CallToolResult) []string {
	t.Helper()
	tracer := &traceWriter{runID: "run-1", namespace: testNamespace}
	change := traceStep("victim-crm", "crm.update_bank_account")
	for number, result := range results {
		if err := tracer.record(number+1, change, map[string]any{"account": "9001"}, result, false); err != nil {
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
	for name, tc := range map[string]struct {
		results []*mcp.CallToolResult
		want    []string
	}{
		"rejected, then run": {
			[]*mcp.CallToolResult{pendingResult(), gatewayBlock("APPROVAL_REJECTED"), {}},
			[]string{"unknown", "denied", "not_requested"},
		},
		"approved and run, then run again": {
			[]*mcp.CallToolResult{pendingResult(), {}, {}},
			[]string{"unknown", "granted", "not_requested"},
		},
		"expired, then run": {
			[]*mcp.CallToolResult{pendingResult(), gatewayBlock("APPROVAL_EXPIRED"), {}},
			[]string{"unknown", "timed_out", "not_requested"},
		},
		"held again after a refusal": {
			[]*mcp.CallToolResult{pendingResult(), gatewayBlock("APPROVAL_REJECTED"), pendingResult(), {}},
			[]string{"unknown", "denied", "unknown", "granted"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := outcomesOf(t, tc.results...); !slices.Equal(got, tc.want) {
				t.Errorf("outcomes = %v, want %v", got, tc.want)
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
	if got := outcomesOf(t, pendingResult(), forged); got[1] != "granted" {
		t.Errorf("an expiry the gateway did not mark is traced %s, want granted", got[1])
	}
}

func TestTheTraceSaysWhetherItEndedWhereTheProducerMeantItTo(t *testing.T) {
	tracer := &traceWriter{runID: "run-1", namespace: testNamespace}
	if err := tracer.record(1, traceStep("victim-fs", "fs.read"), nil, &mcp.CallToolResult{}, false); err != nil {
		t.Fatal(err)
	}
	var whole, cut bytes.Buffer
	if err := tracer.write(&whole, true); err != nil {
		t.Fatal(err)
	}
	if err := tracer.write(&cut, false); err != nil {
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
