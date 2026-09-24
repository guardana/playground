package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func environment(overrides map[string]string) func(string) (string, bool) {
	values := map[string]string{
		"LAB_SERVER_NAME":   "stub-gateway",
		"LAB_LISTEN":        ":8080",
		"LAB_RUN_ID":        "run-1",
		"LAB_REPORTS_DIR":   "/reports",
		"LAB_STUB_VERDICTS": "/config/verdicts.yaml",
		"LAB_UPSTREAMS":     "victim-fs=http://victim-fs:8080/mcp,victim-web=http://victim-web:8080/mcp",
	}
	for name, value := range overrides {
		values[name] = value
	}
	return func(name string) (string, bool) {
		value, present := values[name]
		return value, present
	}
}

func TestConfigReadsTheLabEnvironment(t *testing.T) {
	config, err := readConfig(environment(nil))
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if config.serverName != "stub-gateway" || config.listen != ":8080" {
		t.Errorf("name %q listen %q", config.serverName, config.listen)
	}
	want := filepath.Join("/reports", "run-1", "evidence.jsonl")
	if config.trailPath() != want {
		t.Errorf("trail path is %q, want %q", config.trailPath(), want)
	}
	if len(config.upstreams) != 2 {
		t.Fatalf("%d upstreams, want 2", len(config.upstreams))
	}
	if config.upstreams[0].name != "victim-fs" || config.upstreams[0].endpoint != "http://victim-fs:8080/mcp" {
		t.Errorf("first upstream is %+v", config.upstreams[0])
	}
}

func TestConfigRefusesAMissingVariable(t *testing.T) {
	for _, name := range []string{
		"LAB_SERVER_NAME", "LAB_LISTEN", "LAB_RUN_ID",
		"LAB_REPORTS_DIR", "LAB_STUB_VERDICTS", "LAB_UPSTREAMS",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readConfig(environment(map[string]string{name: ""})); !errors.Is(err, errInvalidConfig) {
				t.Errorf("error is %v, want errInvalidConfig", err)
			}
		})
	}
}

// An upstream the gateway cannot dial is a victim the agent reaches through
// nothing. Refusing at startup says so while a person is still watching.
func TestConfigRefusesAMalformedUpstream(t *testing.T) {
	for _, list := range []string{
		"victim-fs",
		"=http://victim-fs:8080/mcp",
		"victim-fs=",
		"victim-fs=ftp://victim-fs/mcp",
		"victim-fs=http://a/mcp,victim-fs=http://b/mcp",
	} {
		t.Run(list, func(t *testing.T) {
			if _, err := readConfig(environment(map[string]string{"LAB_UPSTREAMS": list})); !errors.Is(err, errInvalidConfig) {
				t.Errorf("error is %v, want errInvalidConfig", err)
			}
		})
	}
}

func TestConfigIgnoresSpacingBetweenUpstreams(t *testing.T) {
	config, err := readConfig(environment(map[string]string{
		"LAB_UPSTREAMS": " victim-fs=http://victim-fs:8080/mcp , victim-db=http://victim-db:8080/mcp ",
	}))
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	if len(config.upstreams) != 2 || config.upstreams[1].name != "victim-db" {
		t.Errorf("upstreams are %+v", config.upstreams)
	}
}
