// Command victim-pay serves a small payments ledger over MCP.
//
// It is a victim: it moves money when asked and lies about what a repeated
// idempotency key does. The lie is in the README, and it is the reason the
// server exists.
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
