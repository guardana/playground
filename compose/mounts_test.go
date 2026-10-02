package compose

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

type mounted struct {
	Services map[string]struct {
		Networks networkList `json:"networks"`
		Profiles []string    `json:"profiles"`
		Volumes  []any       `json:"volumes"`
	} `json:"services"`
}

func readMounts(t *testing.T) mounted {
	t.Helper()
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed mounted
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	return parsed
}

// source reads a volume's source in either spelling, keeping a ${...}
// interpolation whole although it holds a colon.
func source(volume any) string {
	if long, ok := volume.(map[string]any); ok {
		text, _ := long["source"].(string)
		return text
	}
	short, _ := volume.(string)
	if strings.HasPrefix(short, "${") {
		end := strings.Index(short, "}")
		rest, _, _ := strings.Cut(short[end+1:], ":")
		return short[:end+1] + rest
	}
	named, _, _ := strings.Cut(short, ":")
	return named
}

const (
	runDir = "${LAB_RUN_HOST_DIR:-../reports/manual}"
	// workspace is the clone unless the runner names LAB_WORKSPACE.
	workspace = "${LAB_WORKSPACE:-..}"
)

// Each service writes into the part of the run it owns and sees no other: a
// victim that could write the collector's file, or the decision point's CA,
// could forge what the run is graded from.
func TestEachServiceMountsOnlyWhatItWrites(t *testing.T) {
	want := map[string][]string{
		"scripted-agent": {runDir + "/agent", workspace + "/trajectories"},
		"pdp-double":     {runDir + "/journals", workspace + "/config/pdp", "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/pki"},
		"approver":       {runDir + "/journals", workspace + "/config/approver", "approvals"},
		"collector": {"./otel/collector.yaml", "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/collector",
			"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/collector-tls"},
		"trace-verifier":  {"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/verifier", workspace + "/config/contracts"},
		"toxiproxy-tools": {"./toxiproxy/proxies.json"},
		"enforcer": {"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/gateway",
			"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/pki",
			"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/export-ca", "spool", "approvals", "holds"},
	}
	for _, victim := range labspec.Victims() {
		want[victim] = []string{runDir + "/journals"}
	}
	for name, service := range readMounts(t).Services {
		var got []string
		for _, volume := range service.Volumes {
			got = append(got, source(volume))
		}
		expected, pinned := want[name]
		if !pinned {
			if len(got) > 0 && name != "verifier" && name != "attacker-web" {
				t.Errorf("%s mounts %v and this test pins nothing for it", name, got)
			}
			continue
		}
		if !slices.Equal(got, expected) {
			t.Errorf("%s mounts %v, want %v", name, got, expected)
		}
	}
}

func members(t *testing.T, belongs func(networks []string, volumes []string) bool) []string {
	t.Helper()
	var found []string
	for name, service := range readMounts(t).Services {
		var volumes []string
		for _, volume := range service.Volumes {
			volumes = append(volumes, source(volume))
		}
		if belongs(service.Networks, volumes) {
			found = append(found, name)
		}
	}
	slices.Sort(found)
	return found
}

// The agent reaches the enforcer alone, the decision point double only the
// enforcer, and the approver and the trace verifier nothing; the approvals directory is the plane's
// and the approver's alone.
func TestWhoSharesEachNetworkAndTheApprovalsVolume(t *testing.T) {
	on := func(network string) func([]string, []string) bool {
		return func(networks, _ []string) bool { return slices.Contains(networks, network) }
	}
	for what, want := range map[string][]string{
		"agent-net":    {"enforcer", "scripted-agent"},
		"pdp-net":      {"enforcer", "pdp-double"},
		"approver-net": {"approver"},
		"trace-net":    {"trace-verifier"},
	} {
		if got := members(t, on(what)); !slices.Equal(got, want) {
			t.Errorf("%s holds %v, want %v", what, got, want)
		}
	}
	approvals := members(t, func(_, volumes []string) bool { return slices.Contains(volumes, "approvals") })
	if !slices.Equal(approvals, []string{"approver", "enforcer"}) {
		t.Errorf("the approvals volume is mounted by %v, want the enforcer and the approver", approvals)
	}
}

// workspaceMount is a workspace directory bound read-only. A missing source is
// refused rather than created: Docker would create it root-owned, inside the
// adopter's workspace.
func workspaceMount(dir, target string) map[string]any {
	return map[string]any{
		"type": "bind", "source": workspace + dir, "target": target, "read_only": true,
		"bind": map[string]any{"create_host_path": false},
	}
}

// A workspace is the adopter's; no container writes into it or creates a
// directory in it, and each sees only the directory it reads.
func TestTheWorkspaceIsMountedReadOnly(t *testing.T) {
	want := map[string]map[string]any{
		"scripted-agent": workspaceMount("/trajectories", "/trajectories"),
		"pdp-double":     workspaceMount("/config/pdp", "/scripts"),
		"approver":       workspaceMount("/config/approver", "/scripts"),
		"trace-verifier": workspaceMount("/config/contracts", "/contracts"),
	}
	for name, service := range readMounts(t).Services {
		for _, volume := range service.Volumes {
			if !strings.HasPrefix(source(volume), workspace) {
				continue
			}
			if expected, pinned := want[name]; !pinned || !reflect.DeepEqual(volume, any(expected)) {
				t.Errorf("%s mounts %v from the workspace, want %v", name, volume, want[name])
			}
			delete(want, name)
		}
	}
	for name, volume := range want {
		t.Errorf("%s does not mount %s", name, volume)
	}
}
