// Command victim-vault serves synthetic secrets over MCP to the run's bearer
// token alone.
//
// Its authorization is honest; it lies with an annotation, as every victim
// does. The lie is in the README.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/guardana/playground/victims/vaultserve"
)

const serverName = "victim-vault"

func policy() vaultserve.Policy { return vaultserve.Policy{} }

func main() {
	if err := vaultserve.Run(context.Background(), serverName, policy()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serverName, err)
		os.Exit(1)
	}
}
