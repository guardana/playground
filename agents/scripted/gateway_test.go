package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A gateway that speaks streamable HTTP for real, so the transport settings are
// exercised rather than assumed.
func TestReplayThroughStreamableHTTP(t *testing.T) {
	var seen []mcp.Meta
	server := mcp.NewServer(&mcp.Implementation{Name: "stub-gateway", Version: "1"}, nil)
	for _, tool := range []string{"fs.read", "mail.send"} {
		server.AddTool(
			&mcp.Tool{Name: tool, Description: "a tool the lab calls", InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				seen = append(seen, request.Params.Meta)
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "row-1"}}}, nil
			})
	}
	endpoint := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := connect(ctx, endpoint.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	var log bytes.Buffer
	if err := replay(ctx, session, twoSteps(), "run-9", &log); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Errorf("close: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("the server saw %d calls, want 2", len(seen))
	}
	// _meta survives the wire as JSON, so the step number arrives as a number
	// rather than as the int the agent put in.
	if step, ok := seen[1][metaStep].(float64); !ok || int(step) != 2 {
		t.Errorf("the second call carries step %v, want 2", seen[1][metaStep])
	}
	if seen[0][metaRunID] != "run-9" {
		t.Errorf("the first call carries run id %v, want run-9", seen[0][metaRunID])
	}

	// The standalone SSE stream is disabled, so nothing is holding a request
	// open and the server can be closed. Without that, this hangs.
	closed := make(chan struct{})
	go func() {
		endpoint.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("the server would not close; a stream is still open")
	}
}
