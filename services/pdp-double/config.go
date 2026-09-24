package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// maxHold bounds a timeout answer: a held request keeps a connection and a
// goroutine, and the server's write deadline grows with it.
const maxHold = time.Minute

// serverName is stamped on every journal line and names the journal file.
const serverName = "pdp-double"

// errInvalidConfig reports flags or an environment this service will not start on.
var errInvalidConfig = errors.New("pdp-double: invalid configuration")

// settings is what the flags and the lab environment say about one run.
type settings struct {
	listen     string
	identifier string
	base       *url.URL
	sans       []string
	caOut      string
	script     string
	hold       time.Duration
	runID      string
	reportsDir string
}

// parseSettings reads the flags in args and the environment through lookup,
// which is os.LookupEnv outside a test.
func parseSettings(args []string, lookup func(string) (string, bool)) (settings, error) {
	flags := flag.NewFlagSet(serverName, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", ":8443", "address to serve https on")
	identifier := flags.String("identifier", "", "the decision point identifier the gateway is configured with, an https URL")
	san := flags.String("san", "", "comma-separated DNS names and IP addresses the served certificate covers")
	caOut := flags.String("ca-out", "", "file the CA certificate is written to, PEM")
	script := flags.String("script", "", "YAML file scripting the answers")
	hold := flags.Duration("hold", 30*time.Second, "longest a timeout answer holds a request")
	if err := flags.Parse(args); err != nil {
		return settings{}, fmt.Errorf("%w: %w", errInvalidConfig, err)
	}
	if flags.NArg() != 0 {
		return settings{}, fmt.Errorf("%w: unexpected argument %q", errInvalidConfig, flags.Arg(0))
	}
	s := settings{listen: *listen, identifier: *identifier, caOut: *caOut, script: *script, hold: *hold}
	if err := s.readEnv(lookup); err != nil {
		return settings{}, err
	}
	for name, value := range map[string]string{"-listen": s.listen, "-ca-out": s.caOut, "-script": s.script} {
		if strings.TrimSpace(value) == "" {
			return settings{}, fmt.Errorf("%w: %s is empty", errInvalidConfig, name)
		}
	}
	if s.hold <= 0 || s.hold > maxHold {
		return settings{}, fmt.Errorf("%w: -hold %v is not above zero and at most %v", errInvalidConfig, s.hold, maxHold)
	}
	var err error
	if s.base, err = checkIdentifier(s.identifier); err != nil {
		return settings{}, err
	}
	if s.sans, err = splitSANs(*san); err != nil {
		return settings{}, err
	}
	// Control verifies the certificate for the identifier's host; one the SANs
	// miss fails every handshake and reads as PDP_UNAVAILABLE with nothing journalled.
	if !coversHost(s.sans, s.base.Hostname()) {
		return settings{}, fmt.Errorf("%w: -san does not cover the -identifier host %q", errInvalidConfig, s.base.Hostname())
	}
	return s, nil
}

func (s *settings) readEnv(lookup func(string) (string, bool)) error {
	for _, target := range []struct {
		name string
		into *string
	}{{"LAB_RUN_ID", &s.runID}, {"LAB_REPORTS_DIR", &s.reportsDir}} {
		value, _ := lookup(target.name)
		if strings.TrimSpace(value) == "" {
			// A default would journal somewhere the runner does not read, and
			// a missing journal must stay distinguishable from an empty one.
			return fmt.Errorf("%w: %s is not set", errInvalidConfig, target.name)
		}
		*target.into = strings.TrimSpace(value)
	}
	return nil
}

// checkIdentifier accepts what control's client accepts for an https
// identifier: an absolute URL with a host and no userinfo, query or fragment.
func checkIdentifier(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	switch {
	case err != nil || raw == "":
		return nil, fmt.Errorf("%w: -identifier is not a URL", errInvalidConfig)
	case u.Scheme != "https" || u.Host == "":
		return nil, fmt.Errorf("%w: -identifier is not an https URL with a host", errInvalidConfig)
	case u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#"):
		return nil, fmt.Errorf("%w: -identifier carries userinfo, a query or a fragment", errInvalidConfig)
	}
	return u, nil
}

func splitSANs(list string) ([]string, error) {
	var sans []string
	for _, item := range strings.Split(list, ",") {
		if item = strings.TrimSpace(item); item != "" {
			sans = append(sans, item)
		}
	}
	if len(sans) == 0 {
		return nil, fmt.Errorf("%w: -san names no host", errInvalidConfig)
	}
	for _, name := range sans {
		for _, other := range sans {
			if strings.HasSuffix(strings.ToLower(other), "."+strings.ToLower(name)) {
				// The CA excludes everything under each name it permits; a SAN under
				// another would need that exclusion lifted for the whole parent.
				return nil, fmt.Errorf("%w: -san %s sits under -san %s", errInvalidConfig, other, name)
			}
		}
	}
	return sans, nil
}

func (s settings) journalPath() string {
	return filepath.Join(s.reportsDir, s.runID, "journals", serverName+".jsonl")
}

// basePath is the identifier's path, under which the evaluation endpoint and
// the metadata document are published.
func (s settings) basePath() string { return strings.TrimSuffix(s.base.Path, "/") }

func (s settings) evaluationPath() string { return s.basePath() + "/access/v1/evaluation" }

func (s settings) discoveryPath() string {
	return "/.well-known/authzen-configuration" + s.basePath()
}

// endpoint is the evaluation endpoint as control derives it from the
// identifier, which is what its metadata check compares against.
func (s settings) endpoint() string {
	u := *s.base
	u.Path = s.evaluationPath()
	u.RawPath = ""
	return u.String()
}
