package gateway

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// scenarioKeys are the only settings a scenario's part may carry, as the
// dotted paths of the enforcer's own configuration fields at its pin. Every
// other key, one the lab owns or one it leaves at the enforcer's default, is
// refused rather than subtracted: a key the lab forgot to list stays the lab's.
func scenarioKeys() []string {
	return []string{
		"mode", "project_id", "tenant_id", "environment", "log.level",
		"listener.kind", "listener.principal.id", "listener.principal.type", "listener.principal.tenant_id",
		"listener.agent.id", "listener.agent.framework", "listener.agent.version",
		"policy.max_stale", "policy.fail_open_read",
		"pdp.timeout", "pdp.max_in_flight",
		"approvals.provider", "approvals.ttl", "approvals.retry_after", "approvals.max_held",
		"approvals.max_open", "approvals.max_records", "approvals.max_record_bytes", "approvals.reconcile_max",
		"evidence.max_bytes", "evidence.segment_bytes", "evidence.closing_reserve", "evidence.fsync",
		"evidence.fsync_interval", "evidence.on_unwritable",
		"list.shaping", "list.ttl",
		"upstream.call_timeout", "upstream.list_timeout",
	}
}

// identifier is the spelling a tenant or an environment the runner writes into
// an upstream may take.
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// refuseUnallowed walks the scenario's part and names every key it may not
// set. A key holding a dot is refused wherever it sits: the enforcer reads
// `a.b: x` as `a: {b: x}`, so it is another spelling of a key the lab owns.
func refuseUnallowed(config map[string]any) error {
	var refused []string
	walkKeys(config, "", scenarioKeys(), &refused)
	if len(refused) > 0 {
		slices.Sort(refused)
		return fmt.Errorf("%w: the scenario sets %s; a scenario sets only %s",
			ErrInvalid, strings.Join(refused, ", "), strings.Join(scenarioKeys(), ", "))
	}
	return nil
}

func walkKeys(mapping map[string]any, prefix string, allowed []string, refused *[]string) {
	for key, value := range mapping {
		path := prefix + key
		if key == "" || strings.Contains(key, ".") {
			*refused = append(*refused, fmt.Sprintf("%q (a key holding a dot)", path))
			continue
		}
		inner, isMapping := value.(map[string]any)
		switch {
		case slices.Contains(allowed, path) && scalar(value):
		case slices.Contains(allowed, path):
			*refused = append(*refused, path+" (not a single value)")
		case isMapping && hasUnder(allowed, path+"."):
			walkKeys(inner, path+".", allowed, refused)
		default:
			*refused = append(*refused, path)
		}
	}
}

func scalar(value any) bool {
	switch value.(type) {
	case string, bool, float64:
		return true
	}
	return false
}

func hasUnder(allowed []string, prefix string) bool {
	return slices.ContainsFunc(allowed, func(path string) bool { return strings.HasPrefix(path, prefix) })
}

// withTenants returns the upstreams with the tenants the scenario names, and
// refuses a tenant for an upstream the run does not front.
func withTenants(upstreams []Upstream, tenants map[string]string) ([]Upstream, error) {
	placed := slices.Clone(upstreams)
	for name, tenant := range tenants {
		index := slices.IndexFunc(placed, func(u Upstream) bool { return u.Name == name })
		switch {
		case index < 0:
			return nil, fmt.Errorf("%w: a tenant for %s, which the run does not front", ErrInvalid, name)
		case !identifier.MatchString(tenant):
			return nil, fmt.Errorf("%w: the tenant of %s is %q, want letters, digits, _ or -", ErrInvalid, name, tenant)
		}
		placed[index].TenantID = tenant
	}
	return placed, nil
}

// inEnvironment puts every upstream in the plane's own environment, when the
// scenario names one: the enforcer requires a resource's environment to decide
// a write, a delete or a configuration change, and the victims run where the
// plane does.
func inEnvironment(upstreams []Upstream, environment any) ([]Upstream, error) {
	if environment == nil {
		return upstreams, nil
	}
	name, isString := environment.(string)
	if !isString || !identifier.MatchString(name) {
		return nil, fmt.Errorf("%w: environment is %v, want letters, digits, _ or -", ErrInvalid, environment)
	}
	placed := slices.Clone(upstreams)
	for i := range placed {
		placed[i].Environment = name
	}
	return placed, nil
}
