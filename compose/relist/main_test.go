package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pathInput struct {
	Path string `json:"path"`
}

func serverWith(tools ...*mcp.Tool) *httptest.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "listed", Version: "1"}, nil)
	for _, tool := range tools {
		mcp.AddTool(server, tool, func(context.Context, *mcp.CallToolRequest, pathInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{}, nil, nil
		})
	}
	return httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil))
}

// The listing is what a victim that changes its tools on a later listing
// showed this client, so each tool is printed with the description it had.
func TestToolsPrintsEveryToolTheServerListsWithItsDescription(t *testing.T) {
	server := serverWith(
		&mcp.Tool{Name: "fs.read", Description: "Read a file under /data/public."},
		&mcp.Tool{Name: "fs.list", Description: "List a directory."},
	)
	defer server.Close()
	var out strings.Builder
	if err := listTools(server.URL, &out); err != nil {
		t.Fatalf("listTools: %v", err)
	}
	for _, want := range []string{
		`{"name":"fs.list","description":"List a directory."}`,
		`{"name":"fs.read","description":"Read a file under /data/public."}`,
	} {
		if !strings.Contains(out.String(), want+"\n") {
			t.Errorf("the listing lacks %s:\n%s", want, out.String())
		}
	}
}

func TestToolsRefusesAServerThatListsNothing(t *testing.T) {
	server := serverWith()
	defer server.Close()
	if err := listTools(server.URL, &strings.Builder{}); err == nil {
		t.Error("a server listing no tool was read as a listing")
	}
}
