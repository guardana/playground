package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
)

// serverName is stamped on every journal line and names the journal file.
const serverName = "auth-double"

// personaVariable names the persona a scenario chose, a file under -personas
// without its .json suffix.
const personaVariable = labspec.AuthPersonaVariable

// maxPersonaBytes bounds a persona file: a metadata document is a few hundred
// bytes, and the file sits in a workspace an adopter writes.
const maxPersonaBytes = 64 << 10

// errInvalidConfig reports flags or an environment this service will not start on.
var errInvalidConfig = errors.New("auth-double: invalid configuration")

// errNoPersona reports a persona that did not load. It never stops the double:
// it answers 404 instead.
var errNoPersona = errors.New("auth-double: no persona loaded")

// settings is what the flags and the lab environment say about one run.
type settings struct {
	listen     string
	personas   string
	persona    string
	runID      string
	reportsDir string
}

// parseSettings reads the flags in args and the environment through lookup,
// which is os.LookupEnv outside a test.
func parseSettings(args []string, lookup func(string) (string, bool)) (settings, error) {
	flags := flag.NewFlagSet(serverName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", ":8080", "address to serve http on")
	personas := flags.String("personas", "", "directory holding the persona files")
	if err := flags.Parse(args); err != nil {
		return settings{}, fmt.Errorf("%w: %w", errInvalidConfig, err)
	}
	if flags.NArg() != 0 {
		return settings{}, fmt.Errorf("%w: unexpected argument %q", errInvalidConfig, flags.Arg(0))
	}
	s := settings{listen: strings.TrimSpace(*listen), personas: strings.TrimSpace(*personas)}
	if s.listen == "" || s.personas == "" {
		return settings{}, fmt.Errorf("%w: -listen and -personas are required", errInvalidConfig)
	}
	for _, target := range []struct {
		name string
		into *string
	}{{"LAB_RUN_ID", &s.runID}, {"LAB_REPORTS_DIR", &s.reportsDir}} {
		value, _ := lookup(target.name)
		if strings.TrimSpace(value) == "" {
			// A default would journal somewhere the runner does not read, and
			// a missing journal must stay distinguishable from an empty one.
			return settings{}, fmt.Errorf("%w: %s is not set", errInvalidConfig, target.name)
		}
		*target.into = strings.TrimSpace(value)
	}
	persona, _ := lookup(personaVariable)
	s.persona = strings.TrimSpace(persona)
	return s, nil
}

func (s settings) journalPath() string {
	return filepath.Join(s.reportsDir, s.runID, "journals", serverName+".jsonl")
}

// loadPersona reads the persona s names, as written. A document that is not
// JSON does not load: what the double serves is a metadata document that says
// something wrong, never one nobody can read.
func loadPersona(s settings) ([]byte, error) {
	if s.persona == "" {
		return nil, fmt.Errorf("%w: %s is not set", errNoPersona, personaVariable)
	}
	if !plainName(s.persona) {
		return nil, fmt.Errorf("%w: %s %q is not a plain persona name", errNoPersona, personaVariable, s.persona)
	}
	path := filepath.Join(s.personas, s.persona+".json")
	body, err := runfile.ReadRegular(path, maxPersonaBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoPersona, err)
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("%w: %s is not JSON", errNoPersona, path)
	}
	return body, nil
}

// plainName admits lower-case letters, digits and inner hyphens, so a persona
// name can never leave the personas directory.
func plainName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
