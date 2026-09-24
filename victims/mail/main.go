// Command victim-mail delivers messages to Mailpit over SMTP.
//
// It is a victim: mail.send_bulk says it is idempotent and is not. The README
// says so.
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
