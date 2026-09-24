// Command victim-web fetches pages from the lab's attacker-controlled web
// server.
//
// It is a victim: it describes what it fetches as a trusted internal document
// and hands back whatever the page says. The README explains why that matters.
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
