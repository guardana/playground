package main

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// errInvalidConfig reports an environment this service will not start on. It
// stops at the first problem: the reader is looking at a compose file.
var errInvalidConfig = errors.New("stub-gateway: invalid configuration")

// upstreamRef is one victim tool server as the environment names it.
type upstreamRef struct {
	name     string
	endpoint string
}

// config is the lab environment every service reads, plus the two things only
// this one needs: the declared verdicts and the upstreams to sit in front of.
type config struct {
	serverName string
	listen     string
	runID      string
	reportsDir string
	verdicts   string
	upstreams  []upstreamRef
}

// trailPath is where the evidence trail for this run is written. The runner
// reads it from the same path on the host through the bind mount.
func (c config) trailPath() string {
	return filepath.Join(c.reportsDir, c.runID, "evidence.jsonl")
}

// readConfig reads the environment through look, which is os.LookupEnv outside
// a test.
func readConfig(look func(string) (string, bool)) (config, error) {
	values := make(map[string]string, 6)
	for _, name := range []string{
		"LAB_SERVER_NAME", "LAB_LISTEN", "LAB_RUN_ID",
		"LAB_REPORTS_DIR", "LAB_STUB_VERDICTS", "LAB_UPSTREAMS",
	} {
		value, _ := look(name)
		if strings.TrimSpace(value) == "" {
			return config{}, fmt.Errorf("%w: %s is empty", errInvalidConfig, name)
		}
		values[name] = strings.TrimSpace(value)
	}
	upstreams, err := parseUpstreams(values["LAB_UPSTREAMS"])
	if err != nil {
		return config{}, err
	}
	return config{
		serverName: values["LAB_SERVER_NAME"],
		listen:     values["LAB_LISTEN"],
		runID:      values["LAB_RUN_ID"],
		reportsDir: values["LAB_REPORTS_DIR"],
		verdicts:   values["LAB_STUB_VERDICTS"],
		upstreams:  upstreams,
	}, nil
}

// parseUpstreams reads the list the environment carries as
// name=url,name=url. The order is kept, so the trail of a startup failure names
// the same upstream the compose file does.
func parseUpstreams(list string) ([]upstreamRef, error) {
	var refs []upstreamRef
	seen := make(map[string]bool)
	for _, entry := range strings.Split(list, ",") {
		name, endpoint, found := strings.Cut(strings.TrimSpace(entry), "=")
		name, endpoint = strings.TrimSpace(name), strings.TrimSpace(endpoint)
		if !found || name == "" || endpoint == "" {
			return nil, fmt.Errorf("%w: LAB_UPSTREAMS entry %q is not name=url", errInvalidConfig, entry)
		}
		if seen[name] {
			// Two endpoints under one name is a routing question nobody
			// answered, and the trail would name a server the call never
			// reached.
			return nil, fmt.Errorf("%w: LAB_UPSTREAMS names %s twice", errInvalidConfig, name)
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("%w: %s has endpoint %q, want an http URL", errInvalidConfig, name, endpoint)
		}
		seen[name] = true
		refs = append(refs, upstreamRef{name: name, endpoint: endpoint})
	}
	return refs, nil
}
