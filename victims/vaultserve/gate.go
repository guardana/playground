package vaultserve

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/guardana/playground/victims/mcpserve"
)

const (
	// sessionUser binds each session to the token that opened it, so go-sdk
	// refuses a session presented under another identity.
	sessionUser = "lab-operator"
	// tokenLifetime only has to outlast a run: go-sdk refuses a token with no
	// expiration.
	tokenLifetime = time.Hour
)

// gate is the wrapper around the MCP handler: the bearer check, the policy's
// lie, and the auth journal outermost so every decision leaves a line.
func gate(recorder *mcpserve.Recorder, token string, policy Policy, origin string) func(http.Handler) http.Handler {
	bearer := auth.RequireBearerToken(verifier(token, policy), &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: origin + MetadataPath,
		Scopes:              []string{ReadScope},
	})
	return func(next http.Handler) http.Handler {
		checked := bearer(next)
		if !policy.SessionStandsIn {
			return recorder.JournalAuth(token, checked)
		}
		return recorder.JournalAuth(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.Header.Values("Authorization")) == 0 && r.Header.Get("Mcp-Session-Id") != "" {
				next.ServeHTTP(w, r)
				return
			}
			checked.ServeHTTP(w, r)
		}))
	}
}

func verifier(token string, policy Policy) auth.TokenVerifier {
	user := sessionUser
	if policy.SessionStandsIn {
		// A session bound to a user is refused by go-sdk when the request
		// carries no token, which would hide the lie behind the SDK's check.
		user = ""
	}
	return func(_ context.Context, presented string, _ *http.Request) (*auth.TokenInfo, error) {
		if !policy.AcceptAnyToken && subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			Scopes:     []string{ReadScope},
			Expiration: time.Now().Add(tokenLifetime),
			UserID:     user,
		}, nil
	}
}
