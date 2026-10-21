package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/guardana/playground/internal/labspec"
)

// The run's bearer token travels by these names only: the token-gated victims
// read it at boot, and a credential step's container is handed it with -e and
// no value. The double's persona travels the same way.
const (
	mcpTokenVariable    = labspec.MCPTokenVariable
	authPersonaVariable = labspec.AuthPersonaVariable
	tokenBytes          = 32
)

// personaDir is where a run holds the persona its double serves.
const personaDir = "auth"

// handPersona copies the persona the scenario names into the run, where the
// double reads it: a workspace directory is never mounted for it, so a link
// there cannot show the double anything else.
func (l lab) handPersona(spec labspec.Scenario, runDir string) error {
	if spec.AuthDouble == "" {
		return nil
	}
	to := filepath.Join(runDir, personaDir, spec.AuthDouble+".json")
	if err := copyInput(l.workspace.file(personaFile(spec.AuthDouble)), to); err != nil {
		return fmt.Errorf("handing the double persona %s: %w", spec.AuthDouble, err)
	}
	return nil
}

// verifierEnv adds a verifier run's token and the double's persona to env,
// and returns the token; a run through the enforcer gets neither.
func (l lab) verifierEnv(spec labspec.Scenario, env map[string]string) (string, error) {
	if !spec.IsVerifier() {
		return "", nil
	}
	token, err := l.runToken()
	if err != nil {
		return "", fmt.Errorf("minting the run's token: %w", err)
	}
	env[mcpTokenVariable], env[authPersonaVariable] = token, spec.AuthDouble
	return token, nil
}

// runToken mints the token of one verifier run.
func (l lab) runToken() (string, error) {
	if l.token != nil {
		return l.token()
	}
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
