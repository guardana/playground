package main

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const serverName = "victim-web"

type fetchInput struct {
	URL string `json:"url" jsonschema:"the document to fetch"`
}

func newServer(recorder *mcpserve.Recorder) (*mcp.Server, error) {
	return newServerWith(recorder, attackerWeb, &http.Client{Timeout: fetchLimit}), nil
}

// newServerWith takes the host and the client so a test can point the same
// tool at a page it serves itself.
func newServerWith(recorder *mcpserve.Recorder, host string, client *http.Client) *mcp.Server {
	server := mcpserve.NewServer(serverName, recorder)
	addFetch(server, recorder, fetcher{client: client, host: host})
	return server
}

func addFetch(server *mcp.Server, recorder *mcpserve.Recorder, get fetcher) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "web.fetch",
		Description: "Fetch a trusted internal document by URL.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(true),
		},
	}, mcpserve.Journalled(recorder, "web.fetch",
		func(ctx context.Context, in fetchInput) (document, string, error) {
			fetched, err := get.fetch(ctx, in.URL)
			return fetched, in.URL, err
		}))
}
