package gateway_test

import (
	"errors"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/runner/gateway"
)

func inputs(partial string) gateway.Inputs {
	return gateway.Inputs{
		Partial:    []byte(partial),
		Key:        gateway.Key{ID: "ed25519-0011223344556677", Public: "cHVibGljLWtleS1ieXRlcy1mb3ItdGhlLWxhYi10ZXN0cz0="},
		BundleID:   "lab-tool-11",
		BundleFile: "/run/policy.bundle",
		SpoolDir:   "/spool",
		Collector:  "http://collector:4318/v1/logs",
		Upstreams:  []gateway.Upstream{{Name: "victim-crm", Endpoint: "http://victim-crm:8080/mcp"}},
		Overrides: []gateway.Override{{Upstream: "victim-crm", Tool: "crm.read_customer", Fingerprint: "sha256:ff",
			Effect: "READ", ResourceType: "customer", ResourceFrom: "/customer_id"}},
		Listener: "10.231.4.62:8080",
	}
}

const partial = `mode: ENFORCE
project_id: lab
tenant_id: tenant_a
listener:
  kind: stateless_http
  principal: { id: user_123, tenant_id: tenant_a }
  agent: { id: support-agent }
policy: { max_stale: 2m }
`

func TestAssembleJoinsTheScenarioWithWhatTheLabOwns(t *testing.T) {
	out, err := gateway.Assemble(inputs(partial))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	var got struct {
		Mode      string              `json:"mode"`
		Listener  map[string]any      `json:"listener"`
		Health    map[string]string   `json:"health"`
		Policy    map[string]string   `json:"policy"`
		Evidence  map[string]string   `json:"evidence"`
		Export    map[string]any      `json:"export"`
		Upstreams []map[string]string `json:"upstreams"`
		Overrides []map[string]string `json:"overrides"`
	}
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatalf("the assembled file does not read back: %v\n%s", err, out)
	}
	checks := map[string][2]string{
		"mode":               {got.Mode, "ENFORCE"},
		"listener.address":   {str(got.Listener["address"]), "10.231.4.62:8080"},
		"listener.kind":      {str(got.Listener["kind"]), "stateless_http"},
		"health.address":     {got.Health["address"], "127.0.0.1:8081"},
		"policy.bundle_id":   {got.Policy["bundle_id"], "lab-tool-11"},
		"policy.bundle_file": {got.Policy["bundle_file"], "/run/policy.bundle"},
		"policy.key_id":      {got.Policy["key_id"], "ed25519-0011223344556677"},
		"policy.max_stale":   {got.Policy["max_stale"], "2m"},
		"evidence.dir":       {got.Evidence["dir"], "/spool"},
		"export.endpoint":    {str(got.Export["endpoint"]), "http://collector:4318/v1/logs"},
		"upstreams.0.name":   {got.Upstreams[0]["name"], "victim-crm"},
		"overrides.0.fp":     {got.Overrides[0]["fingerprint"], "sha256:ff"},
	}
	for name, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
	if got.Export["in_flight"] != "1" || got.Export["allow_plaintext"] != "true" {
		t.Errorf("export = %v, want in_flight 1 over plaintext on the sealed network", got.Export)
	}
}

func TestAssembleWritesOnlyTheSubsetTheEnforcerReads(t *testing.T) {
	in := inputs(partial + "list: { shaping: none }\n")
	out, err := gateway.Assemble(in)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	for number, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		switch {
		case strings.ContainsAny(strings.SplitN(trimmed, "\"", 2)[0], "{}[]&*|>"):
			t.Errorf("line %d is not plain block YAML: %q", number+1, line)
		case strings.HasPrefix(trimmed, "- ") && indent == 0:
			t.Errorf("line %d is a sequence item at the key's own indent: %q", number+1, line)
		case strings.Contains(trimmed, ": ") && !strings.Contains(trimmed, ": \""):
			t.Errorf("line %d holds an unquoted scalar: %q", number+1, line)
		}
	}
	if !strings.Contains(string(out), "upstreams:\n  -\n    endpoint: \"http://victim-crm:8080/mcp\"\n") {
		t.Errorf("the upstreams are not a block sequence under their key:\n%s", out)
	}
}

func str(value any) string {
	text, _ := value.(string)
	return text
}

func TestAssembleRefusesAScenarioSettingWhatTheLabOwns(t *testing.T) {
	for _, extra := range []string{
		"health: { address: 0.0.0.0:9000 }\n",
		"export: { endpoint: http://elsewhere/v1/logs }\n",
		"upstreams: [ { name: x, endpoint: http://x/mcp } ]\n",
		"overrides: []\n",
		"evidence: { dir: /tmp }\n",
	} {
		if _, err := gateway.Assemble(inputs(partial + extra)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("a scenario setting %q was assembled: %v", strings.TrimSpace(extra), err)
		}
	}
	for _, replaced := range []string{"policy: { max_stale: 2m, key_id: other }", "policy: { max_stale: 2m, bundle_file: /x }"} {
		body := strings.Replace(partial, "policy: { max_stale: 2m }", replaced, 1)
		if _, err := gateway.Assemble(inputs(body)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("a scenario setting %q was assembled: %v", replaced, err)
		}
	}
	listenerAddress := strings.Replace(partial, "  kind: stateless_http", "  kind: stateless_http\n  address: 0.0.0.0:1", 1)
	if _, err := gateway.Assemble(inputs(listenerAddress)); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("a scenario moving the listener was assembled: %v", err)
	}
	if _, err := gateway.Assemble(inputs("policy: strict\n")); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("a policy that is not a mapping was assembled: %v", err)
	}
	if _, err := gateway.Assemble(inputs("")); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("an empty scenario configuration was assembled: %v", err)
	}
	missing := inputs(partial)
	missing.Key.ID = ""
	if _, err := gateway.Assemble(missing); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("a configuration without a key id was assembled: %v", err)
	}
}

func TestParseKeyReadsKeygensTwoLines(t *testing.T) {
	key, err := gateway.ParseKey([]byte("key_id: ed25519-3802f1da55221936\npublic_key: KwT1Y0coqVbWZI6s0Ho+Qt34Ea35dT6MrGyD7Ombk6Y=\n"))
	if err != nil || key.ID != "ed25519-3802f1da55221936" || key.Public != "KwT1Y0coqVbWZI6s0Ho+Qt34Ea35dT6MrGyD7Ombk6Y=" {
		t.Errorf("ParseKey = %+v, %v", key, err)
	}
	for _, bad := range []string{"", "key_id: a\n", "public_key: b\n", "key_id: a\npublic_key: b\nsigning_key: c\n", "key_id a\n"} {
		if _, err := gateway.ParseKey([]byte(bad)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("ParseKey(%q) = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestBundleIDIsTheDocumentsOwn(t *testing.T) {
	id, err := gateway.BundleID([]byte(`{"apiVersion":"agent-policy/v1alpha1","bundle":{"id":"lab-tool-11","version":"1","serial":1}}`))
	if err != nil || id != "lab-tool-11" {
		t.Errorf("BundleID = %q, %v", id, err)
	}
	for _, bad := range []string{`{}`, `{"bundle":{}}`, `not json`} {
		if _, err := gateway.BundleID([]byte(bad)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("BundleID(%s) = %v, want ErrInvalid", bad, err)
		}
	}
}

func TestOverridesPinEveryClassifiedToolToOneFingerprint(t *testing.T) {
	classes := []gateway.Class{
		{Upstream: "victim-crm", Tool: "crm.read_customer", Effect: "READ", ResourceType: "customer"},
		{Upstream: "victim-shell", Tool: "shell.exec", Effect: "EXECUTE", ResourceType: "command"},
	}
	prints := []gateway.Fingerprint{
		{Upstream: "victim-crm", Tool: "crm.read_customer", Fingerprint: "sha256:aa"},
		{Upstream: "victim-shell", Tool: "shell.exec", Fingerprint: "sha256:bb"},
	}
	got, err := gateway.Overrides(classes, prints, []string{"victim-shell/shell.exec"})
	if err != nil || len(got) != 1 || got[0].Fingerprint != "sha256:aa" || got[0].Effect != "READ" {
		t.Fatalf("Overrides = %+v, %v", got, err)
	}
	if _, err := gateway.Overrides(classes, prints[:1], nil); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("a classified tool with no fingerprint was pinned: %v", err)
	}
	if _, err := gateway.Overrides(classes, append(prints, prints[0]), nil); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("a classified tool with two fingerprints was pinned: %v", err)
	}
	if _, err := gateway.Overrides(classes, prints, []string{"victim-web/web.fetch"}); !errors.Is(err, gateway.ErrInvalid) {
		t.Errorf("an unknown tool was kept unclassified: %v", err)
	}
}

func TestAssembleGivesTheFileProviderAndTheDoubleTheLabsPlaces(t *testing.T) {
	in := inputs(partial + "approvals:\n  provider: file\n  ttl: 30s\npdp:\n  timeout: 150ms\n")
	in.UsesPDP = true
	out, err := gateway.Assemble(in)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	for _, want := range []string{
		`  dir: "/var/lib/lab/approvals"`, `  hold_journal_dir: "/var/lib/lab/holds"`, `  ttl: "30s"`,
		`  identifier: "https://pdp-double:8443"`, `  timeout: "150ms"`,
	} {
		if !strings.Contains(string(out), want+"\n") {
			t.Errorf("the configuration does not hold %s:\n%s", want, out)
		}
	}
	memory, err := gateway.Assemble(inputs(partial + "approvals:\n  provider: memory\n"))
	if err != nil || strings.Contains(string(memory), "hold_journal_dir") {
		t.Errorf("a memory provider was given the lab's directories: %v\n%s", err, memory)
	}
	for _, extra := range []string{"approvals:\n  dir: /tmp\n", "pdp:\n  identifier: https://elsewhere\n"} {
		if _, err := gateway.Assemble(inputs(partial + extra)); !errors.Is(err, gateway.ErrInvalid) {
			t.Errorf("a scenario setting %q was assembled: %v", extra, err)
		}
	}
}
