// Command victim-db serves raw SQL over two in-memory tables.
//
// It is a victim: the statement parser is naive on purpose, one tool describes
// itself as a read-only report and writes, and the README says so.
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
