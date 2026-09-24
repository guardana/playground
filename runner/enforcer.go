package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/gateway"
)

// What the lab owns in the enforcer's configuration, as the compose topology
// mounts and names it.
const (
	gatewayDirName   = "gateway"
	gatewayMount     = "/gateway"
	enforcerSpoolDir = "/var/lib/lab/spool"
	collectorLogs    = "http://collector:4318/v1/logs"
	classification   = "config/gateway/classification.yaml"
	fingerprints     = "config/gateway/fingerprints.yaml"
)

// victims are the upstreams every enforcer run fronts.
func victims() []string {
	return []string{"victim-crm", "victim-db", "victim-fs", "victim-shell", "victim-mail", "victim-web"}
}

// gatewayHost is the service the agent talks to: the enforcer, or the stub.
func gatewayHost(spec labspec.Scenario) string {
	if spec.UsesEnforcer() {
		return enforcerService
	}
	return gatewayService
}

// sealedFromAgent are the services a run boots that the agent must not reach.
func sealedFromAgent(spec labspec.Scenario) []string {
	if !spec.UsesEnforcer() {
		return nil
	}
	sealed := []string{"collector:4318", "enforcer:8081"}
	if spec.Gateway.PDPScript != "" {
		sealed = append(sealed, "pdp-double:8443")
	}
	if spec.Gateway.ApproverScript != "" {
		sealed = append(sealed, "approver:8080")
	}
	return sealed
}

// prepareEnforcer writes the run's gateway directory and makes the ones the
// collector and the decision point double write into.
func (l lab) prepareEnforcer(ctx context.Context, spec labspec.Scenario, runDir string) error {
	for _, shared := range []string{"collector", "pki"} {
		if err := makeShared(filepath.Join(runDir, shared)); err != nil {
			return err
		}
	}
	routes, err := l.routedUpstreams(spec.Proxied())
	if err != nil {
		return err
	}
	_, err = l.prepareGateway(ctx, spec.Gateway, routes, runDir)
	return err
}

// unprepared is the report of a run whose enforcer could not be prepared: no
// key, a policy that does not sign, a configuration the lab refuses. Nothing
// was brought up, and the run fails rather than skipping.
func unprepared(id, runID string, cause error, at time.Time) assertion.Report {
	return assertion.Report{
		Scenario: id, RunID: runID, StartedAt: at, EndedAt: at,
		Results: []assertion.Result{{
			Check: "plane/prepared", Outcome: assertion.Fail,
			Want: "a signed bundle and an assembled configuration for the enforcer",
			Got:  "the run was refused before anything was brought up", Detail: cause.Error(),
		}},
	}
}

// prepareGateway writes the run's gateway directory: the bundle signed for
// this run and the configuration assembled around it. It returns the
// directory, which compose mounts read-only into the enforcer.
func (l lab) prepareGateway(
	ctx context.Context, plan *labspec.Gateway, routes []gateway.Upstream, runDir string,
) (string, error) {
	if err := l.refuseKeysPlace(); err != nil {
		return "", err
	}
	dir := filepath.Join(runDir, gatewayDirName)
	if err := os.Mkdir(dir, 0o755); err != nil { // #nosec G301,G703 -- inside the run directory; the enforcer's uid reads it, nothing in it is secret.
		return "", err
	}
	lines, err := os.ReadFile(filepath.Join(l.keysDir, "public.txt")) // #nosec G304 -- the lab key's public half.
	if err != nil {
		return "", fmt.Errorf("no lab key at %s (run make lab-key): %w", l.keysDir, err)
	}
	key, err := gateway.ParseKey(lines)
	if err != nil {
		return "", err
	}
	document, err := os.ReadFile(filepath.Join(l.root, plan.Policy)) // #nosec G304 -- a repository path the scenario names.
	if err != nil {
		return "", err
	}
	bundleID, err := gateway.BundleID(document)
	if err != nil {
		return "", err
	}
	if err := l.signInto(ctx, filepath.Join(l.root, plan.Policy), dir); err != nil {
		return "", fmt.Errorf("signing %s: %w", plan.Policy, err)
	}
	overrides, err := l.overrides(plan.Unclassified)
	if err != nil {
		return "", err
	}
	partial, err := os.ReadFile(filepath.Join(l.root, plan.Config)) // #nosec G304 -- a repository path the scenario names.
	if err != nil {
		return "", err
	}
	assembled, err := gateway.Assemble(gateway.Inputs{
		Partial: partial, Key: key, BundleID: bundleID, BundleFile: gatewayMount + "/policy.bundle",
		SpoolDir: enforcerSpoolDir, Collector: collectorLogs, Upstreams: routes, Overrides: overrides,
		UsesPDP: plan.PDPScript != "", UpstreamTenants: plan.UpstreamTenants,
	})
	if err != nil {
		return "", err
	}
	return dir, os.WriteFile(filepath.Join(dir, "gateway.yaml"), assembled, 0o644) // #nosec G306,G703 -- public configuration, inside the run directory.
}

func (l lab) overrides(unclassified []string) ([]gateway.Override, error) {
	var classes []gateway.Class
	var prints []gateway.Fingerprint
	for path, into := range map[string]any{classification: &classes, fingerprints: &prints} {
		body, err := os.ReadFile(filepath.Join(l.root, path)) // #nosec G304 -- the lab's own files.
		if err != nil {
			return nil, err
		}
		if err := yaml.UnmarshalStrict(body, into); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return gateway.Overrides(classes, prints, unclassified)
}
