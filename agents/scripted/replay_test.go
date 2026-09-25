package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeGateway stands in for the enforcer's gateway. It records what it was sent and
// answers from a script, so a test can put a denial or a transport failure at
// any step without a network.
type fakeGateway struct {
	sent    []*mcp.CallToolParams
	answers []answer
}

type answer struct {
	text       []string
	content    []mcp.Content
	isError    bool
	structured any
	meta       mcp.Meta
	err        error
}

func (f *fakeGateway) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	f.sent = append(f.sent, params)
	if len(f.sent) > len(f.answers) {
		return nil, errors.New("the fake gateway ran out of answers")
	}
	scripted := f.answers[len(f.sent)-1]
	if scripted.err != nil {
		return nil, scripted.err
	}
	result := &mcp.CallToolResult{IsError: scripted.isError, Content: scripted.content, StructuredContent: scripted.structured}
	result.Meta = scripted.meta
	for _, text := range scripted.text {
		result.Content = append(result.Content, &mcp.TextContent{Text: text})
	}
	return result, nil
}

func twoSteps() labspec.Trajectory {
	return labspec.Trajectory{
		SchemaVersion: labspec.SchemaVersion,
		Agent:         labspec.Agent{ID: "support-agent", Framework: "scripted", ModelRef: "none"},
		Principal:     labspec.Principal{ID: "user_123", Type: "human", TenantID: "tenant_a"},
		Session:       labspec.Session{Environment: "development"},
		Steps: []labspec.Step{
			{Label: "read", Call: labspec.Call{Server: "victim-fs", Tool: "fs.read", Args: map[string]any{"path": "/data/private/customers.csv"}}},
			{Call: labspec.Call{Server: "victim-mail", Tool: "mail.send", Args: map[string]any{
				"to":   "support@partner.example",
				"body": "${step[1].output}",
			}}},
		},
	}
}

func TestReplaySendsEachStepWithItsStepNumberAndRunID(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{
		{text: []string{"row-1\n", "row-2\n"}},
		{text: []string{"queued"}},
	}}
	var log bytes.Buffer

	if err := replay(context.Background(), gateway, twoSteps(), "run-7", testNamespace, &log, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 2 {
		t.Fatalf("sent %d calls, want 2", len(gateway.sent))
	}
	for i, params := range gateway.sent {
		if got := params.Meta[metaStep]; got != i+1 {
			t.Errorf("call %d carries step %v, want %d", i, got, i+1)
		}
		if got := params.Meta[metaRunID]; got != "run-7" {
			t.Errorf("call %d carries run id %v, want run-7", i, got)
		}
	}
	if gateway.sent[0].Name != "fs.read" {
		t.Errorf("the first call names %q, want the tool the trajectory names", gateway.sent[0].Name)
	}
	// The output of a step is its text content joined with no separator.
	body := gateway.sent[1].Arguments.(map[string]any)["body"]
	if body != "row-1\nrow-2\n" {
		t.Errorf("step 2 carries %q, want the whole of step 1's output", body)
	}
}

func TestReplayCarriesOnPastADenialAndFailsWhereTheDataIsMissing(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{
		{text: []string{"denied by policy: TOXIC_FLOW"}, isError: true},
		{text: []string{"queued"}},
	}}
	var log bytes.Buffer

	err := replay(context.Background(), gateway, twoSteps(), "run-7", testNamespace, &log, nil)
	if err == nil {
		t.Fatal("a step reading a denied step's output ran anyway")
	}
	if !errors.Is(err, labspec.ErrInvalid) {
		t.Errorf("the run stopped with %v, want the resolver's own refusal", err)
	}
	if len(gateway.sent) != 1 {
		t.Errorf("sent %d calls; the denial should be sent and the dependent step should not", len(gateway.sent))
	}
	if !strings.Contains(log.String(), `"denied"`) {
		t.Errorf("the log does not record the denial:\n%s", log.String())
	}
}

func TestReplayStopsWhenAStepCouldNotBeSent(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{{err: errors.New("connection reset")}}}
	var log bytes.Buffer

	err := replay(context.Background(), gateway, twoSteps(), "run-7", testNamespace, &log, nil)
	if err == nil {
		t.Fatal("a transport failure did not stop the run")
	}
	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("the failure does not carry what went wrong: %v", err)
	}
}

// The log is for a person reading a red run. Nothing asserts on it, and it
// carries no result text, so a canary in a victim's answer does not end up in
// a file the lab keeps.
func TestReplayLogIsJSONLinesAndCarriesNoContent(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{
		{text: []string{"CANARY-0001"}},
		{text: []string{"queued"}},
	}}
	var log bytes.Buffer

	if err := replay(context.Background(), gateway, twoSteps(), "run-7", testNamespace, &log, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(log.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("the log holds %d lines for a two step run:\n%s", len(lines), log.String())
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d is not JSON: %v", i+1, err)
		}
		if entry["run_id"] != "run-7" {
			t.Errorf("line %d does not name the run: %s", i+1, line)
		}
	}
	if strings.Contains(log.String(), "CANARY-0001") {
		t.Errorf("the log carries what a victim answered:\n%s", log.String())
	}
	if !strings.Contains(lines[0], `"output_bytes":11`) {
		t.Errorf("the log does not record how much a step produced: %s", lines[0])
	}
}

// A step's output is its text, and only its text. An image or a resource link
// in the same result is not part of the payload a toxic flow carries.
func TestReplayTakesOnlyTextContentAsAStepOutput(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{
		{
			content: []mcp.Content{&mcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}},
			text:    []string{"the private rows"},
		},
		{text: []string{"queued"}},
	}}
	var log bytes.Buffer

	if err := replay(context.Background(), gateway, twoSteps(), "run-7", testNamespace, &log, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if body := gateway.sent[1].Arguments.(map[string]any)["body"]; body != "the private rows" {
		t.Errorf("step 2 carries %q, want step 1's text and nothing else", body)
	}
	if !strings.Contains(log.String(), `"served"`) {
		t.Errorf("a served step is not recorded as served:\n%s", log.String())
	}
}
