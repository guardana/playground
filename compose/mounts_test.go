package compose

import (
	"os"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

type mounted struct {
	Services map[string]struct {
		Networks []string `json:"networks"`
		Profiles []string `json:"profiles"`
		Volumes  []any    `json:"volumes"`
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

const runDir = "${LAB_RUN_HOST_DIR:-../reports/manual}"

// Each service writes into the part of the run it owns and sees no other: a
// victim that could write the collector's file, or the decision point's CA,
// could forge what the run is graded from.
func TestEachServiceMountsOnlyWhatItWrites(t *testing.T) {
	want := map[string][]string{
		"victim-crm": {runDir + "/journals"}, "victim-db": {runDir + "/journals"},
		"victim-fs": {runDir + "/journals"}, "victim-shell": {runDir + "/journals"},
		"victim-mail": {runDir + "/journals"}, "victim-web": {runDir + "/journals"},
		"scripted-agent": {runDir + "/agent", "../trajectories"},
		"stub-gateway":   {runDir, "../config"},
		"pdp-double":     {runDir + "/journals", "../config/pdp", "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/pki"},
		"approver":       {runDir + "/journals", "../config/approver", "approvals"},
		"collector":      {"./otel/collector.yaml", "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/collector"},
		"enforcer": {"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/gateway",
			"${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/pki", "spool", "approvals", "holds"},
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

// The agent reaches one gateway, the decision point double only the
// enforcer, and the approver nothing; the approvals directory is the plane's
// and the approver's alone.
func TestWhoSharesEachNetworkAndTheApprovalsVolume(t *testing.T) {
	on := func(network string) func([]string, []string) bool {
		return func(networks, _ []string) bool { return slices.Contains(networks, network) }
	}
	for what, want := range map[string][]string{
		"agent-net":    {"enforcer", "scripted-agent", "stub-gateway"},
		"pdp-net":      {"enforcer", "pdp-double"},
		"approver-net": {"approver"},
	} {
		if got := members(t, on(what)); !slices.Equal(got, want) {
			t.Errorf("%s holds %v, want %v", what, got, want)
		}
	}
	approvals := members(t, func(_, volumes []string) bool { return slices.Contains(volumes, "approvals") })
	if !slices.Equal(approvals, []string{"approver", "enforcer"}) {
		t.Errorf("the approvals volume is mounted by %v, want the enforcer and the approver", approvals)
	}
	services := readMounts(t).Services
	for _, profile := range services["stub-gateway"].Profiles {
		if slices.Contains(services["enforcer"].Profiles, profile) {
			t.Errorf("profile %s starts both gateways, so the agent would reach two", profile)
		}
	}
}
