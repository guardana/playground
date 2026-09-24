package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/evidence"
)

// hangGuard stops a hung handler outliving the test that started it. It is far
// longer than the deadline under test, so it never decides the outcome.
const hangGuard = 2 * time.Second

// A victim that accepts a call and never answers leaves the trail at
// ACTION_STARTED, and the chain rule reads a trail ending there as a request in
// flight rather than as a defect, so the hang is graded as silence. The deadline
// is what turns it into ACTION_FAILED, the record the contract has for an action
// that did not finish.
func TestAHungUpstreamIsRecordedAsAFailedAction(t *testing.T) {
	served := &atomic.Int64{}
	victim := mcp.NewServer(&mcp.Implementation{Name: "victim-fs", Version: "test"}, nil)
	victim.AddTool(lyingTool(), func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		served.Add(1)
		select {
		case <-ctx.Done():
		case <-time.After(hangGuard):
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "answered too late"}}}, nil
	})

	file := &syncBuffer{}
	gateway := newGateway("stub-gateway", declaredFor(t, "  - verdict: ALLOW\n    reason_codes: [RULE_ALLOW]\n"),
		newTrail(file, runIdentity{runID: "run-1", projectID: "project-1", tenantID: "tenant-1"}))
	gateway.callTimeout = 50 * time.Millisecond
	server, err := gateway.newServer(t.Context(),
		[]upstream{{name: "victim-fs", session: connect(t, victim, "stub-gateway")}})
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	hung := &lab{client: connect(t, server, "scripted-agent"), served: served, file: file}

	started := time.Now()
	if _, err := hung.client.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "fs.read",
		Arguments: map[string]any{"path": "/data/private/customers.csv"},
		Meta:      mcp.Meta{metaStepKey: 1},
	}); err == nil {
		t.Error("a call to a victim that never answered came back without an error")
	}
	if waited := time.Since(started); waited >= hangGuard {
		t.Errorf("the gateway waited %s on a hung upstream, so nothing bounded the call", waited)
	}

	events := trailOf(t, hung)
	want := []evidence.Kind{
		evidence.KindActionProposed,
		evidence.KindPolicyDecided,
		evidence.KindActionStarted,
		evidence.KindActionFailed,
	}
	if !slices.Equal(kinds(events), want) {
		t.Fatalf("trail is %v, want %v", kinds(events), want)
	}
	if last := events[len(events)-1]; last.Result == nil || last.Result.EndedAt == nil {
		t.Error("the failed action carries no end time, so the trail does not say when it stopped")
	}
}

// A victim that accepts the connection and never completes the handshake would
// otherwise hang startup with nothing written down: the gateway has not opened
// its trail yet, and compose is still waiting on a health check that cannot
// answer.
func TestDiallingAnUpstreamThatNeverAnswersGivesUp(t *testing.T) {
	stalled := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(hangGuard):
		}
	}))
	t.Cleanup(stalled.Close)

	settings := config{
		serverName: "stub-gateway",
		upstreams:  []upstreamRef{{name: "victim-fs", endpoint: stalled.URL + "/mcp"}},
	}

	started := time.Now()
	_, err := dialUpstreams(t.Context(), settings, 50*time.Millisecond)
	if err == nil {
		t.Fatal("a victim that never answered was connected to")
	}
	if waited := time.Since(started); waited >= hangGuard {
		t.Errorf("dialling took %s, so nothing bounded the handshake", waited)
	}
	if !strings.Contains(err.Error(), "victim-fs") {
		t.Errorf("the failure does not name the upstream: %v", err)
	}
}
