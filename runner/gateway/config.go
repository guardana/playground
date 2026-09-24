// Package gateway assembles the enforcer's configuration for one scenario run:
// the scenario's own part (mode, principal, approvals, decision point timing)
// joined with what the lab owns (addresses, the signed bundle and its key, the
// spool, the collector, the upstreams and their classification). A scenario's
// part may set only the keys scenarioKeys lists, so no scenario can point the
// plane at another bundle, key, collector or upstream than the run built.
package gateway

import (
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

// ErrInvalid reports input this package will not assemble a configuration from.
var ErrInvalid = errors.New("gateway: invalid")

// The plane's directories and the double's address, as the compose topology
// mounts and names them.
const (
	approvalsDir  = "/var/lib/lab/approvals"
	holdsDir      = "/var/lib/lab/holds"
	PDPIdentifier = "https://pdp-double:8443"
)

// Inputs is everything one run's configuration is made from.
type Inputs struct {
	Partial    []byte
	Key        Key
	BundleID   string
	BundleFile string
	SpoolDir   string
	Collector  string
	Upstreams  []Upstream
	Overrides  []Override
	// UsesPDP points the plane at the decision point double.
	UsesPDP bool
	// UpstreamTenants puts named upstreams in a tenant of their own.
	UpstreamTenants map[string]string
}

// Upstream is one victim the gateway fronts.
type Upstream struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	TenantID string `json:"tenant_id,omitempty"`
}

// Override classifies one tool definition, pinned by its fingerprint.
type Override struct {
	Upstream     string `json:"upstream"`
	Tool         string `json:"tool"`
	Fingerprint  string `json:"fingerprint"`
	Effect       string `json:"effect"`
	ResourceType string `json:"resource_type"`
	ResourceFrom string `json:"resource_from,omitempty"`
	TrustZone    string `json:"trust_zone,omitempty"`
}

// Assemble returns the configuration file the gateway runs with.
func Assemble(in Inputs) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(in.Partial, &config); err != nil {
		return nil, fmt.Errorf("%w: the scenario's gateway configuration: %w", ErrInvalid, err)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: the scenario's gateway configuration is empty", ErrInvalid)
	}
	if err := refuseUnallowed(config); err != nil {
		return nil, err
	}
	if err := in.check(); err != nil {
		return nil, err
	}
	upstreams, err := withTenants(in.Upstreams, in.UpstreamTenants)
	if err != nil {
		return nil, err
	}
	listener, policy, evidence, err := sections(config)
	if err != nil {
		return nil, err
	}
	listener["address"] = "0.0.0.0:8080"
	config["health"] = map[string]any{"address": "127.0.0.1:8081"}
	policy["bundle_id"], policy["bundle_file"] = in.BundleID, in.BundleFile
	policy["key_id"], policy["public_key"] = in.Key.ID, in.Key.Public
	evidence["dir"] = in.SpoolDir
	config["export"] = map[string]any{"endpoint": in.Collector, "allow_plaintext": true, "in_flight": 1}
	config["upstreams"] = upstreams
	if len(in.Overrides) > 0 {
		config["overrides"] = in.Overrides
	}
	placeDoubles(config, in.UsesPDP)
	return emit(config)
}

// placeDoubles gives the file approval provider the plane's two directories and
// points the decision point at the double, where the scenario uses them.
func placeDoubles(config map[string]any, usesPDP bool) {
	if approvals, ok := config["approvals"].(map[string]any); ok && approvals["provider"] == "file" {
		approvals["dir"], approvals["hold_journal_dir"] = approvalsDir, holdsDir
	}
	if !usesPDP {
		return
	}
	pdp, ok := config["pdp"].(map[string]any)
	if !ok {
		pdp = map[string]any{}
		config["pdp"] = pdp
	}
	pdp["identifier"] = PDPIdentifier
}

func (in Inputs) check() error {
	for name, value := range map[string]string{
		"key id": in.Key.ID, "public key": in.Key.Public, "bundle id": in.BundleID,
		"bundle file": in.BundleFile, "spool": in.SpoolDir, "collector": in.Collector,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: no %s", ErrInvalid, name)
		}
	}
	if len(in.Upstreams) == 0 {
		return fmt.Errorf("%w: no upstream", ErrInvalid)
	}
	return nil
}

func sections(config map[string]any) (listener, policy, evidence map[string]any, err error) {
	found := make([]map[string]any, 0, 3)
	for _, name := range []string{"listener", "policy", "evidence"} {
		value, present := config[name]
		inner, isMap := value.(map[string]any)
		switch {
		case !present:
			inner = map[string]any{}
			config[name] = inner
		case !isMap:
			return nil, nil, nil, fmt.Errorf("%w: %s is not a mapping", ErrInvalid, name)
		}
		found = append(found, inner)
	}
	return found[0], found[1], found[2], nil
}
