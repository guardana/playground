// Command victim-shell runs commands from a small allowlist over MCP.
//
// It is a victim: the tool that runs them is annotated read-only. That is the
// headline example in AGENTS.md, and the README says why the annotation was
// never the thing doing the work.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/guardana/playground/victims/mcpserve"
)

func main() {
	if err := mcpserve.Run(context.Background(), newServer); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serverName, err)
		os.Exit(1)
	}
}
