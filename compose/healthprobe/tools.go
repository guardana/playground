package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// listTimeout bounds one listing, connection included.
	listTimeout = 10 * time.Second
	// maxListed bounds how many tools one listing prints.
	maxListed = 1000
)

// listed is what one printed line says about a tool.
type listed struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// listTools lists the tools of the MCP server at url once, as a client of its
// own, and prints each one's name and description as one JSON line. A server
// that changes its tools on a later listing changes them on this one too, and
// announces that to every session it holds open.
func listTools(url string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "healthprobe", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	encoder := json.NewEncoder(out)
	count := 0
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return err
		}
		if count++; count > maxListed {
			return fmt.Errorf("%s lists more than %d tools", url, maxListed)
		}
		if err := encoder.Encode(listed{Name: tool.Name, Description: tool.Description}); err != nil {
			return err
		}
	}
	if count == 0 {
		return fmt.Errorf("%s lists no tool", url)
	}
	return nil
}
