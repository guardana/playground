// Package vaultserve is the part of a token-gated vault that is not its lie.
//
// Three victims serve the same secrets behind the same bearer-token check, the
// same protected resource metadata and the same auth journal. They differ in
// one Policy field each, so a verifier finding against one of them names that
// field and nothing else.
package vaultserve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/victims/mcpserve"
)

const (
	// TokenVariable names the environment variable holding the run's bearer
	// token. The runner mints it per run and hands it over at boot.
	TokenVariable = labspec.MCPTokenVariable
	// AuthorizationServer is the authorization-server double on tool-net.
	AuthorizationServer = "http://auth-double:8080"
	// MetadataPath is where a vault publishes its protected resource metadata.
	MetadataPath = "/.well-known/oauth-protected-resource"
	// ReadScope is the scope the challenge names and the run's token carries.
	ReadScope = "vault.read"
	// WriteScope is advertised alongside it.
	WriteScope = "vault.write"
	// port is the one LAB_LISTEN gives every victim in compose: the resource
	// the metadata declares has to be the origin the verifier dials.
	port = "8080"
)

// ErrNoToken is the refusal to start without the run's token. A vault that
// came up with an empty token would gate on nothing.
var ErrNoToken = errors.New("vaultserve: " + TokenVariable + " is not set")

// Policy is what a vault lies about. The zero value is honest authorization.
type Policy struct {
	// AcceptAnyToken serves any bearer token, not only the run's.
	AcceptAnyToken bool
	// SessionStandsIn serves a request that carries a session id and no
	// credential, as if the session were the credential.
	SessionStandsIn bool
}

// Run is the whole life of a vault: read the run's token, refuse to start
// without one, then serve as mcpserve.RunRouted does.
func Run(ctx context.Context, name string, policy Policy) error {
	token := os.Getenv(TokenVariable)
	if token == "" {
		return ErrNoToken
	}
	return mcpserve.RunRouted(ctx, func(recorder *mcpserve.Recorder) (*mcp.Server, mcpserve.Routing, error) {
		return Build(recorder, name, token, policy)
	})
}

// Build makes the vault's MCP server and its routing: /mcp behind the bearer
// check and the auth journal, and the protected resource metadata beside it.
func Build(recorder *mcpserve.Recorder, name, token string, policy Policy) (*mcp.Server, mcpserve.Routing, error) {
	if token == "" {
		return nil, mcpserve.Routing{}, ErrNoToken
	}
	server := mcpserve.NewServer(name, recorder)
	server.AddReceivingMiddleware(privateListings)
	addTools(server, recorder, newStore())

	origin := Origin(name)
	routing := mcpserve.Routing{
		Wrap: gate(recorder, token, policy, origin),
		Routes: map[string]http.Handler{
			MetadataPath: auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
				Resource:             origin,
				AuthorizationServers: []string{AuthorizationServer},
				ScopesSupported:      []string{ReadScope, WriteScope},
			}),
		},
	}
	return server, routing, nil
}

// Origin is the address a vault named name answers at on tool-net.
func Origin(name string) string {
	return fmt.Sprintf("http://%s:%s", name, port)
}

// privateListings marks the tool listing as one user's, never a shared
// cache's: what a gated server lists depends on who asked.
func privateListings(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, request)
		if listed, ok := result.(*mcp.ListToolsResult); ok {
			listed.CacheScope = "private"
		}
		return result, err
	}
}
