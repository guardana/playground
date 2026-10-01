package gateway_test

import (
	"errors"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/runner/gateway"
)

func TestOverridesCarryWhatEachToolReturns(t *testing.T) {
	classes := []gateway.Class{
		{Upstream: "victim-fs", Tool: "fs.read", Effect: "READ", ResourceType: "file",
			Returns: &gateway.Returns{Trust: "TRUSTED_INTERNAL", Sensitivity: "CONFIDENTIAL"}},
		{Upstream: "victim-shell", Tool: "shell.exec", Effect: "EXECUTE", ResourceType: "command"},
	}
	prints := []gateway.Fingerprint{
		{Upstream: "victim-fs", Tool: "fs.read", Fingerprint: "sha256:aa"},
		{Upstream: "victim-shell", Tool: "shell.exec", Fingerprint: "sha256:bb"},
	}
	overrides, err := gateway.Overrides(classes, prints, nil)
	if err != nil {
		t.Fatalf("Overrides: %v", err)
	}
	in := inputs(partial)
	in.Overrides = overrides
	out, err := gateway.Assemble(in)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	var got struct {
		Overrides []struct {
			Tool    string            `json:"tool"`
			Returns map[string]string `json:"returns"`
		} `json:"overrides"`
	}
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("the assembled file does not read back: %v\n%s", err, out)
	}
	if len(got.Overrides) != 2 {
		t.Fatalf("overrides = %+v, want two", got.Overrides)
	}
	if read := got.Overrides[0]; read.Tool != "fs.read" || read.Returns["trust"] != "TRUSTED_INTERNAL" ||
		read.Returns["sensitivity"] != "CONFIDENTIAL" {
		t.Errorf("fs.read's override = %+v, want returns trust TRUSTED_INTERNAL, sensitivity CONFIDENTIAL\n%s", read, out)
	}
	if shell := got.Overrides[1]; shell.Returns != nil {
		t.Errorf("shell.exec declares no returns and its override carries %v", shell.Returns)
	}
}

func TestOverridesRefuseAReturnsThatStatesNothing(t *testing.T) {
	classes := []gateway.Class{{Upstream: "victim-fs", Tool: "fs.read", Effect: "READ", ResourceType: "file",
		Returns: &gateway.Returns{}}}
	prints := []gateway.Fingerprint{{Upstream: "victim-fs", Tool: "fs.read", Fingerprint: "sha256:aa"}}
	if _, err := gateway.Overrides(classes, prints, nil); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("an empty returns was accepted: %v", err)
	}
}
