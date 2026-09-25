package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRunRefusesAReplayThatIsMissingAFlag(t *testing.T) {
	tests := []struct {
		args    []string
		missing string
	}{
		{nil, "-trajectory"},
		{[]string{"-trajectory", "trajectories/x.yaml"}, "-gateway"},
		{[]string{"-trajectory", "trajectories/x.yaml", "-gateway", "http://gateway/mcp"}, "-run-id"},
		{[]string{"-trajectory", "trajectories/x.yaml", "-gateway", "http://gateway/mcp", "-run-id", "r"}, "-namespace"},
		{[]string{"-trajectory", "trajectories/x.yaml", "-gateway", "http://gateway/mcp", "-run-id", "r", "-namespace", "n"}, "-out"},
	}
	for _, test := range tests {
		var out strings.Builder
		err := run(context.Background(), test.args, &out)
		if err == nil {
			t.Errorf("%v was accepted as a whole invocation", test.args)
			continue
		}
		if !strings.Contains(err.Error(), test.missing+" is required") {
			t.Errorf("%v failed with %v, want it to name %s", test.args, err, test.missing)
		}
	}
}

func TestRunProbesWithoutTheReplayFlags(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	var out strings.Builder
	if err := run(context.Background(), []string{"-probe", listener.Addr().String()}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), probeReached) {
		t.Errorf("the probe wrote %q", out.String())
	}
}

func TestRunReplaysATrajectoryFromDiskAndWritesItsLog(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "enforcer", Version: "1"}, nil)
	server.AddTool(
		&mcp.Tool{Name: "fs.read", Description: "a tool the lab calls", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "row-1"}}}, nil
		})
	endpoint := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil))
	defer endpoint.Close()

	directory := t.TempDir()
	trajectory := filepath.Join(directory, "one-step.yaml")
	body := `schema_version: 1
agent: { id: support-agent, framework: scripted, model_ref: none }
principal: { id: user_123, type: human, tenant_id: tenant_a }
session: { environment: development }
steps:
  - call:
      server: victim-fs
      tool: fs.read
      args: { path: "/data/private/customers.csv" }
`
	if err := os.WriteFile(trajectory, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(directory, "journals", "agent.jsonl")

	var out strings.Builder
	err := run(context.Background(), []string{
		"-trajectory", trajectory,
		"-gateway", endpoint.URL,
		"-run-id", "run-11",
		"-namespace", "guardana.control",
		"-out", log,
	}, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	written, err := os.ReadFile(log) // #nosec G304 -- the path the test just built.
	if err != nil {
		t.Fatalf("the agent wrote no log: %v", err)
	}
	if !strings.Contains(string(written), `"step":1`) {
		t.Errorf("the log does not record the step:\n%s", written)
	}
}
