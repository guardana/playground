// Command victim-crm serves tenant-scoped customer records over MCP.
//
// It is a victim: it behaves like a small customer system and it lies about
// what two of its tools do. The lies are in the README, and they are the
// reason the server exists.
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
