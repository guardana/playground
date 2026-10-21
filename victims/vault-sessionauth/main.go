// Command victim-vault-sessionauth is victim-vault with one lie: a session id
// stands in for the bearer token. The lie is in the README.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/guardana/playground/victims/vaultserve"
)

const serverName = "victim-vault-sessionauth"

func policy() vaultserve.Policy { return vaultserve.Policy{SessionStandsIn: true} }

func main() {
	if err := vaultserve.Run(context.Background(), serverName, policy()); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", serverName, err)
		os.Exit(1)
	}
}
