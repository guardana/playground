// Command victim-fs serves a sandboxed filesystem over MCP.
//
// It is a victim: fs.read describes itself differently on the second listing
// than on the first. The sandbox itself is real, and the README says which is
// which.
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
