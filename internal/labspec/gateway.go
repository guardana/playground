package labspec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Gateway names what the enforcer decides a trajectory scenario with: the
// scenario's part of the enforcer's configuration, the policy document the
// runner signs with the lab key for the run, and the tools it keeps
// unclassified on purpose. The runner adds everything else and refuses a part
// that sets a key it does not let a scenario set.
//
// PDPScript and ApproverScript name the scripts under config/pdp/ and
// config/approver/ that the decision point double and the approver answer by;
// each needs its compose profile, and neither runs without its script.
type Gateway struct {
	Config         string   `json:"config"`
	Policy         string   `json:"policy"`
	Unclassified   []string `json:"unclassified,omitempty"`
	PDPScript      string   `json:"pdp_script,omitempty"`
	ApproverScript string   `json:"approver_script,omitempty"`
	// UpstreamTenants puts a victim in a tenant of its own, for a scenario
	// that crosses tenants.
	UpstreamTenants map[string]string `json:"upstream_tenants,omitempty"`
}

// The directories the two files live in.
const (
	GatewayConfigDir = "config/gateway/scenarios/"
	PolicyDir        = "config/policies/"
)

var (
	upstreamTool = regexp.MustCompile(`^victim-[a-z]+/[a-z_]+\.[a-z_]+$`)
	upstreamName = regexp.MustCompile(`^victim-[a-z]+$`)
	tenantName   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// UsesEnforcer reports whether the enforcer decides the scenario: every
// trajectory scenario does, and no verifier scenario.
func (s Scenario) UsesEnforcer() bool { return s.Gateway != nil }

// validateDecider refuses a trajectory scenario that names nothing to decide it.
func (s Scenario) validateDecider() error {
	if s.Gateway == nil {
		return fmt.Errorf("%w: gateway is not set, so nothing would decide the run", ErrInvalid)
	}
	if err := s.Gateway.validate(); err != nil {
		return err
	}
	return s.validateProfiles()
}

func (g Gateway) validate() error {
	switch {
	case !underWith(g.Config, GatewayConfigDir, ".yaml"):
		return fmt.Errorf("%w: gateway.config is %q, want a .yaml file under %s", ErrInvalid, g.Config, GatewayConfigDir)
	case !underWith(g.Policy, PolicyDir, ".json"):
		return fmt.Errorf("%w: gateway.policy is %q, want a .json file under %s", ErrInvalid, g.Policy, PolicyDir)
	}
	for i, name := range g.Unclassified {
		if !upstreamTool.MatchString(name) || slices.Contains(g.Unclassified[:i], name) {
			return fmt.Errorf("%w: gateway.unclassified[%d] is %q, want a distinct victim-<name>/<tool>", ErrInvalid, i, name)
		}
	}
	for upstream, tenant := range g.UpstreamTenants {
		if !upstreamName.MatchString(upstream) || !tenantName.MatchString(tenant) {
			return fmt.Errorf("%w: gateway.upstream_tenants names %q in %q, want victim-<name>: letters, digits, _ or -",
				ErrInvalid, upstream, tenant)
		}
	}
	return nil
}

// underWith reports whether path is a file of that suffix inside directory.
func underWith(path, directory, suffix string) bool {
	return strings.HasPrefix(path, directory) && strings.HasSuffix(path, suffix) && !strings.Contains(path, "..")
}

// validateProfiles ties the profile to the enforcer that decides the run, and
// each double's profile to its script.
func (s Scenario) validateProfiles() error {
	if !slices.Contains(s.Profile, "enforcer") {
		return fmt.Errorf("%w: the enforcer decides this run, so profile names enforcer", ErrInvalid)
	}
	for _, double := range []struct{ profile, script string }{
		{"pdp", s.Gateway.PDPScript}, {"approvals", s.Gateway.ApproverScript},
	} {
		if err := validateDouble(s.Profile, double.profile, double.script); err != nil {
			return err
		}
	}
	return nil
}

func validateDouble(profiles []string, profile, script string) error {
	named := script != ""
	if named != slices.Contains(profiles, profile) {
		return fmt.Errorf("%w: profile %s and its script go together", ErrInvalid, profile)
	}
	if named && (strings.ContainsAny(script, "/\\") || !strings.HasSuffix(script, ".yaml")) {
		return fmt.Errorf("%w: the %s script %q is a .yaml file name, not a path", ErrInvalid, profile, script)
	}
	return nil
}
