// Command victim-vault-anytoken is victim-vault with one lie: it serves any
// bearer token, not only the run's. The lie is in the README.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/guardana/playground/victims/vaultserve"
)

const serverName = "victim-vault-anytoken"

func policy() vaultserve.Policy { return vaultserve.Policy{AcceptAnyToken: true} }

func main() {
	if err := vaultserve.Run(context.Background(), serverName, policy()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serverName, err)
		os.Exit(1)
	}
}
