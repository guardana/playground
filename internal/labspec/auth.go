package labspec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/journal"
)

// AuthPersonaDir is where the authorization-server double's personas live.
const AuthPersonaDir = "config/auth/"

// The variables a verifier run's token and the double's persona travel by, the
// one name the runner, compose, the token-gated victims and the double share.
const (
	MCPTokenVariable    = "LAB_MCP_TOKEN" // #nosec G101 -- a variable's name, never its value.
	AuthPersonaVariable = "LAB_AUTH_PERSONA"
)

// AuthExpectation is one authentication decision a token-gated victim has to
// have journalled during the run: what the request carried and, when stated,
// the status it answered. It is read from the victim's own journal, so a claim
// about the token is checked against a record the verifier did not write.
type AuthExpectation struct {
	Credential string `json:"credential"`
	Session    string `json:"session,omitempty"`
	Status     int    `json:"status,omitempty"`
}

// validateAuth refuses a persona the double could not serve to this run, and an
// auth expectation no journal line could meet.
func (s Scenario) validateAuth() error {
	if s.AuthDouble != "" {
		switch {
		case !s.IsVerifier():
			return fmt.Errorf("%w: auth_double is set, and only a verifier scenario boots the double", ErrInvalid)
		case !plainName(s.AuthDouble):
			return fmt.Errorf("%w: auth_double is %q, want a persona name in %s without its .json", ErrInvalid, s.AuthDouble, AuthPersonaDir)
		}
	}
	for _, server := range sortedKeys(s.Expect.Effects) {
		wants := s.Expect.Effects[server].AuthInclude
		if len(wants) > 0 && !slices.Contains(VerifierVictims(), server) {
			return fmt.Errorf("%w: expect.effects.%s.auth_include is stated and only a token-gated victim journals its authentication",
				ErrInvalid, server)
		}
		for i, want := range wants {
			if err := want.validate(fmt.Sprintf("expect.effects.%s.auth_include[%d]", server, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a AuthExpectation) validate(field string) error {
	switch {
	case !journal.Credential(a.Credential).Known():
		return fmt.Errorf("%w: %s.credential is %q, want none, run or other", ErrInvalid, field, a.Credential)
	case a.Session != "" && !journal.Session(a.Session).Known():
		return fmt.Errorf("%w: %s.session is %q, want none, known or unknown", ErrInvalid, field, a.Session)
	case a.Status != 0 && (a.Status < 100 || a.Status > 599):
		return fmt.Errorf("%w: %s.status is %d, want an HTTP status", ErrInvalid, field, a.Status)
	}
	return nil
}

// plainName admits lower-case letters, digits and inner hyphens, so a name can
// never leave the directory it is looked up in.
func plainName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' })
}

// Matches reports whether one recorded decision is this one: the same
// credential, and the same session and status where they are stated.
func (a AuthExpectation) Matches(decision journal.Auth) bool {
	return string(decision.Credential) == a.Credential &&
		(a.Session == "" || string(decision.Session) == a.Session) &&
		(a.Status == 0 || decision.Status == a.Status)
}
