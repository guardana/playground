package control

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/drivers/control/gateway"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/internal/runfile"
	"github.com/guardana/playground/runner/driver"
)

// What the lab owns in the enforcer's configuration, as the compose topology
// mounts and names it.
const (
	gatewayDirName   = "gateway"
	gatewayMount     = "/gateway"
	enforcerSpoolDir = "/var/lib/lab/spool"
	collectorLogs    = "https://" + collectorHost + ":4318/v1/logs"
	classification   = "config/gateway/classification.yaml"
	fingerprints     = "config/gateway/fingerprints.yaml"
)

// sealedFromAgent are the services a run boots that the agent must not reach.
func sealedFromAgent(spec labspec.Scenario) []string {
	sealed := []string{"collector:4318", "enforcer:8081"}
	if spec.Gateway.PDPScript != "" {
		sealed = append(sealed, "pdp-double:8443")
	}
	if spec.Gateway.ApproverScript != "" {
		sealed = append(sealed, "approver:8080")
	}
	return sealed
}

// prepareEnforcer writes the run's gateway directory and the collector's TLS
// material, and makes the directories the collector and the decision point
// double write into.
func (d *Driver) prepareEnforcer(ctx context.Context, spec labspec.Scenario, run driver.Run) error {
	runDir := run.Dir
	for _, shared := range []string{"collector", "pki"} {
		if err := makeShared(filepath.Join(runDir, shared)); err != nil {
			return err
		}
	}
	routes, err := d.routedUpstreams(spec.Proxied())
	if err != nil {
		return err
	}
	if _, err := d.prepareGateway(ctx, spec.Gateway, routes, run); err != nil {
		return err
	}
	if err := writeCollectorTLS(runDir, d.clock()); err != nil {
		return errors.Join(fmt.Errorf("the collector's certificate: %w", err), dropCollectorKey(runDir))
	}
	return nil
}

// prepareGateway writes the run's gateway directory: the bundle signed for
// this run and the configuration assembled around it. It returns the
// directory, which compose mounts read-only into the enforcer.
func (d *Driver) prepareGateway(
	ctx context.Context, plan *labspec.Gateway, routes []gateway.Upstream, run driver.Run,
) (string, error) {
	if err := d.refuseKeysPlace(); err != nil {
		return "", err
	}
	dir := filepath.Join(run.Dir, gatewayDirName)
	if err := os.Mkdir(dir, 0o755); err != nil { // #nosec G301,G703 -- inside the run directory; the enforcer's uid reads it, nothing in it is secret.
		return "", err
	}
	lines, err := os.ReadFile(filepath.Join(d.config.KeysDir, "public.txt")) // #nosec G304 -- the lab key's public half.
	if err != nil {
		return "", fmt.Errorf("no lab key at %s (run make lab-key): %w", d.config.KeysDir, err)
	}
	key, err := gateway.ParseKey(lines)
	if err != nil {
		return "", err
	}
	document, err := os.ReadFile(d.config.Workspace.file(plan.Policy)) // #nosec G304 -- a workspace path the scenario names.
	if err != nil {
		return "", err
	}
	bundleID, err := gateway.BundleID(document)
	if err != nil {
		return "", err
	}
	if err := d.signInto(ctx, d.config.Workspace.file(plan.Policy), dir); err != nil {
		return "", fmt.Errorf("signing %s: %w", plan.Policy, err)
	}
	overrides, err := d.overrides(plan.Unclassified)
	if err != nil {
		return "", err
	}
	partial, err := os.ReadFile(d.config.Workspace.file(plan.Config)) // #nosec G304 -- a workspace path the scenario names.
	if err != nil {
		return "", err
	}
	assembled, err := gateway.Assemble(gateway.Inputs{
		Partial: partial, Key: key, BundleID: bundleID, BundleFile: gatewayMount + "/policy.bundle",
		SpoolDir: enforcerSpoolDir, Collector: collectorLogs, Upstreams: routes, Overrides: overrides,
		UsesPDP: plan.PDPScript != "", UpstreamTenants: plan.UpstreamTenants,
		Listener: net.JoinHostPort(run.GateAddress, servicePort),
	})
	if err != nil {
		return "", err
	}
	// Public configuration the enforcer's uid reads.
	return dir, runfile.CreateNew(filepath.Join(dir, "gateway.yaml"), 0o644, func(file *os.File) error {
		_, err := file.Write(assembled)
		return err
	})
}

func (d *Driver) overrides(unclassified []string) ([]gateway.Override, error) {
	var classes []gateway.Class
	var prints []gateway.Fingerprint
	for path, into := range map[string]any{classification: &classes, fingerprints: &prints} {
		body, err := os.ReadFile(filepath.Join(d.config.Root, path)) // #nosec G304 -- the lab's own files.
		if err != nil {
			return nil, err
		}
		if err := yaml.UnmarshalStrict(body, into); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return gateway.Overrides(classes, prints, unclassified)
}
