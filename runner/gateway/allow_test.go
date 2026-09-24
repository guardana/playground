package gateway_test

import (
	"errors"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/runner/gateway"
)

// The enforcer reads `a.b: x` as `a: {b: x}`, so a dotted key is another
// spelling of a nested one and could set whatever the lab owns.
func TestAssembleRefusesAKeyHoldingADotAtAnyDepth(t *testing.T) {
	for name, extra := range map[string]string{
		"an injected override":    "\"overrides.14.effect\": READ\n\"overrides.14.tool\": mail.send\n",
		"an injected upstream":    "\"upstreams.6.command\": /enforcer/control\n",
		"the decision point":      "\"pdp.identifier\": https://victim-web:8080\n",
		"an allowed key, dotted":  "\"policy.max_stale\": 1m\n",
		"nested one level":        "approvals:\n  \"hold_journal_dir.x\": /tmp\n",
		"nested under the policy": "evidence:\n  \"on_unwritable.y\": block\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := gateway.Assemble(inputs(partial + extra))
			if !errors.Is(err, gateway.ErrInvalid) || !strings.Contains(err.Error(), "dot") {
				t.Errorf("assembled or refused for another reason: %v", err)
			}
		})
	}
}

// Every key the lab lets a scenario set is accepted, one at a time.
func TestAssembleAcceptsEveryKeyAScenarioMaySet(t *testing.T) {
	for _, path := range []string{
		"mode", "project_id", "tenant_id", "environment", "log.level",
		"listener.kind", "listener.principal.id", "listener.principal.type", "listener.principal.tenant_id",
		"listener.agent.id", "listener.agent.framework", "listener.agent.version",
		"policy.max_stale", "policy.fail_open_read", "pdp.timeout", "pdp.max_in_flight",
		"approvals.provider", "approvals.ttl", "approvals.retry_after", "approvals.max_held", "approvals.max_open",
		"approvals.max_records", "approvals.max_record_bytes", "approvals.reconcile_max",
		"evidence.max_bytes", "evidence.segment_bytes", "evidence.closing_reserve", "evidence.fsync",
		"evidence.fsync_interval", "evidence.on_unwritable", "list.shaping", "list.ttl",
		"upstream.call_timeout", "upstream.list_timeout",
	} {
		if _, err := gateway.Assemble(inputs(nested(path, "x"))); err != nil {
			t.Errorf("%s was refused: %v", path, err)
		}
	}
}

// Anything else is refused, the enforcer's own keys the lab owns or leaves at
// their defaults included, and a value where a mapping or a scalar belongs.
func TestAssembleRefusesEveryOtherKey(t *testing.T) {
	for _, path := range []string{
		"listener.address", "listener.origins", "health.address", "policy.bundle_id", "policy.public_key",
		"pdp.identifier", "pdp.evaluation_endpoint", "pdp.allow_plaintext", "pdp.proxy",
		"pdp.informational_context", "approvals.dir", "approvals.hold_journal_dir", "evidence.dir",
		"export.timeout", "upstreams", "overrides", "unknown", "listener.principal.id.more",
	} {
		if _, err := gateway.Assemble(inputs(nested(path, "x"))); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("%s was assembled: %v", path, err)
		}
	}
	for name, body := range map[string]string{
		"a list for a scalar":    "mode: [ENFORCE]\n",
		"no value":               "mode:\n",
		"a scalar for a mapping": "listener: stateless_http\n",
		"a mapping for a scalar": "mode: { name: ENFORCE }\n",
	} {
		if _, err := gateway.Assemble(inputs(body)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("%s was assembled: %v", name, err)
		}
	}
}

// nested spells a dotted path as the block mappings it stands for.
func nested(path, value string) string {
	var out strings.Builder
	parts := strings.Split(path, ".")
	for depth, part := range parts {
		out.WriteString(strings.Repeat("  ", depth) + part + ":")
		if depth == len(parts)-1 {
			out.WriteString(" " + value)
		}
		out.WriteString("\n")
	}
	return out.String()
}

func TestAssembleGivesAnUpstreamTheTenantTheScenarioNames(t *testing.T) {
	in := inputs(partial)
	in.Upstreams = append(in.Upstreams, gateway.Upstream{Name: "victim-fs", Endpoint: "http://victim-fs:8080/mcp"})
	in.UpstreamTenants = map[string]string{"victim-crm": "tenant_b"}
	out, err := gateway.Assemble(in)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	var got struct {
		Upstreams []map[string]string `json:"upstreams"`
	}
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Upstreams[0]["tenant_id"] != "tenant_b" || got.Upstreams[1]["tenant_id"] != "" {
		t.Errorf("upstreams = %v, want victim-crm alone in tenant_b", got.Upstreams)
	}
	for name, tenants := range map[string]map[string]string{
		"an upstream the run does not front": {"victim-nope": "tenant_b"},
		"an empty tenant":                    {"victim-crm": ""},
		"a tenant with a newline":            {"victim-crm": "tenant_b\nmode: OBSERVE"},
	} {
		in.UpstreamTenants = tenants
		if _, err := gateway.Assemble(in); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("%s was assembled: %v", name, err)
		}
	}
}

func TestAssemblePutsEveryUpstreamInThePlanesEnvironment(t *testing.T) {
	in := inputs(partial + "environment: development\n")
	in.Upstreams = append(in.Upstreams, gateway.Upstream{Name: "victim-fs", Endpoint: "http://victim-fs:8080/mcp"})
	for body, want := range map[string]string{partial + "environment: development\n": "development", partial: ""} {
		in.Partial = []byte(body)
		out, err := gateway.Assemble(in)
		if err != nil {
			t.Fatalf("Assemble: %v", err)
		}
		var got struct {
			Upstreams []map[string]string `json:"upstreams"`
		}
		if err := yaml.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		if got.Upstreams[0]["environment"] != want || got.Upstreams[1]["environment"] != want {
			t.Errorf("upstreams = %v, want each in environment %q", got.Upstreams, want)
		}
	}
	for _, environment := range []string{"true", "\"dev\\nmode: OBSERVE\"", "\"\""} {
		in.Partial = []byte(partial + "environment: " + environment + "\n")
		if _, err := gateway.Assemble(in); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("environment %s was assembled: %v", environment, err)
		}
	}
}
