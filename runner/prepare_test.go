package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const partialFile = "config/gateway/scenarios/flow-01.yaml"

func TestAScenarioPartSettingWhatTheLabOwnsIsRefusedBeforeAnythingBoots(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	path := filepath.Join(subject.root, partialFile)
	body, err := os.ReadFile(path) // #nosec G304 -- the test's own file.
	if err != nil {
		t.Fatal(err)
	}
	writeFile(path, string(body)+"overrides.1.effect: READ\noverrides.1.tool: fs.read\n")
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/prepared"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "dot") {
		t.Errorf("a part injecting an override was %s: %+v", result.Outcome, result)
	}
	if len(compose.broughtUp) != 0 {
		t.Errorf("a refused run brought up %v", compose.broughtUp)
	}
}

func TestAnUpstreamTenantReachesTheAssembledConfiguration(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	body, err := os.ReadFile(scenario) // #nosec G304 -- the test's own file.
	if err != nil {
		t.Fatal(err)
	}
	writeFile(scenario, strings.Replace(string(body), "policy: config/policies/flow-01.json }",
		"policy: config/policies/flow-01.json, upstream_tenants: { victim-crm: tenant_b } }", 1))
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	assembled, err := os.ReadFile(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "gateway", "gateway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(assembled), "    name: \"victim-crm\"\n    tenant_id: \"tenant_b\"\n") {
		t.Errorf("victim-crm is not in tenant_b:\n%s", assembled)
	}
}

// The signer writes where it likes; the run's gateway directory, which the
// enforcer mounts, receives the bundle and nothing else.
func TestTheSignerWritesOutsideTheRunAndOnlyTheBundleIsKept(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	var signedInto string
	subject.sign = func(_ context.Context, _, _, outDir string) error {
		signedInto = outDir
		writeFile(filepath.Join(outDir, "policy.bundle"), "signed")
		writeFile(filepath.Join(outDir, "gateway.yaml"), "mode: OBSERVE\n")
		writeFile(filepath.Join(outDir, "planted"), "x")
		return nil
	}
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	runDir := compose.env["LAB_RUN_HOST_DIR"]
	if signedInto == "" || strings.HasPrefix(signedInto, runDir) {
		t.Errorf("the signer wrote into %q, inside the run %q", signedInto, runDir)
	}
	if _, err := os.Stat(signedInto); !os.IsNotExist(err) {
		t.Errorf("the signer's directory %q was left behind: %v", signedInto, err)
	}
	entries, err := os.ReadDir(filepath.Join(runDir, "gateway"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if !slices.Equal(names, []string{"gateway.yaml", "policy.bundle"}) {
		t.Errorf("the gateway directory holds %v", names)
	}
	assembled, _ := os.ReadFile(filepath.Join(runDir, "gateway", "gateway.yaml")) // #nosec G304 -- the test's run.
	if strings.Contains(string(assembled), "OBSERVE") {
		t.Errorf("the signer's configuration replaced the assembled one:\n%s", assembled)
	}
}

// A key in a workspace directory a container mounts would be read by that
// container, the agent included.
func TestALabKeyInsideTheCloneTheReportsOrTheWorkspaceIsRefused(t *testing.T) {
	for _, place := range []string{"clone", "reports", "workspace"} {
		t.Run(place, func(t *testing.T) {
			subject, compose, scenario := enforcerLab(t)
			inside := filepath.Join(subject.root, "keys")
			switch place {
			case "reports":
				inside = filepath.Join(subject.reports, "keys")
			case "workspace":
				apart(t, &subject, classification, fingerprints, versionFile)
				subject.workspace.external = true
				inside = filepath.Join(subject.workspace.dir, "trajectories", "keys")
			}
			writeFile(filepath.Join(inside, "public.txt"), "key_id: ed25519-0011223344556677\npublic_key: cHVibGljLWtleQ==\n")
			subject.keysDir = inside
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if result := results(graded)["plane/prepared"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "LAB_KEYS_DIR") {
				t.Errorf("a key in the %s was %s: %+v", place, result.Outcome, result)
			}
			if len(compose.broughtUp) != 0 {
				t.Errorf("a refused run brought up %v", compose.broughtUp)
			}
		})
	}
}

func TestTheSignerRunsUnprivilegedFromThePinnedImageOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(filepath.Join(root, versionFile), "ENFORCER_IMAGE=lab-enforcer\nENFORCER_COMMIT="+testPin+"\n")
	d := &daemon{writer: ownIDs()}
	sign := signWithEnforcer(root, d.run)
	if err := sign(context.Background(), "/keys", "/repo/config/policies/p.json", "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(d.calls[len(d.calls)-1], " ")
	for _, want := range []string{
		"--pull never", "--cap-drop ALL", "--security-opt no-new-privileges", "--network none", "--read-only",
		"-v /keys:/key:ro", "-v /tmp/out:/out", "lab-enforcer:" + testPin,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the signer ran without %q: %s", want, joined)
		}
	}
}
