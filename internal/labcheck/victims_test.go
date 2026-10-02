package labcheck_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/gateway"
)

// The victim list lives in every file that boots, routes, proxies or
// classifies a victim. labspec.Victims is the one each is held to, so a victim
// added in one place and forgotten in another fails here rather than as a
// scenario that quietly never reaches it.
func TestEveryCopyOfTheVictimListAgrees(t *testing.T) {
	want := slices.Sorted(slices.Values(labspec.Victims()))
	services, toxiproxyWaits, enforcerWaits := composeVictims(t)
	// Each of these names a victim once; a second entry, such as a second
	// proxy for one victim, is a disagreement and is not compacted away.
	for place, got := range map[string][]string{
		"compose/compose.yaml services":                   services,
		"compose/compose.yaml toxiproxy-tools depends_on": toxiproxyWaits,
		"compose/compose.yaml enforcer depends_on":        enforcerWaits,
		"compose/toxiproxy/proxies.json":                  proxyNames(t),
		"scripts/classify-victims.sh":                     scriptVictims(t),
		"victims/*/main.go":                               victimDirs(t),
		"config/gateway/tools/*.json":                     snapshotNames(t),
	} {
		if got = slices.Sorted(slices.Values(got)); !slices.Equal(got, want) {
			t.Errorf("%s names %v, want %v", place, got, want)
		}
	}
	// The classification and the fingerprints hold one line per tool.
	for place, got := range map[string][]string{
		"config/gateway/classification.yaml": classifiedUpstreams(t),
		"config/gateway/fingerprints.yaml":   fingerprintedUpstreams(t),
	} {
		if got = slices.Compact(slices.Sorted(slices.Values(got))); !slices.Equal(got, want) {
			t.Errorf("%s names %v, want %v", place, got, want)
		}
	}
}

func composeVictims(t *testing.T) (services, toxiproxyWaits, enforcerWaits []string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "compose", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Services map[string]struct {
			DependsOn map[string]json.RawMessage `json:"depends_on"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal(body, &file); err != nil {
		t.Fatalf("compose/compose.yaml: %v", err)
	}
	victims := func(names []string) []string {
		return slices.DeleteFunc(names, func(name string) bool { return !strings.HasPrefix(name, "victim-") })
	}
	services = victims(slices.Collect(maps.Keys(file.Services)))
	toxiproxyWaits = victims(slices.Collect(maps.Keys(file.Services["toxiproxy-tools"].DependsOn)))
	enforcerWaits = victims(slices.Collect(maps.Keys(file.Services["enforcer"].DependsOn)))
	return services, toxiproxyWaits, enforcerWaits
}

func proxyNames(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "compose", "toxiproxy", "proxies.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proxies []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &proxies); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		names = append(names, proxy.Name)
	}
	return names
}

func scriptVictims(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "classify-victims.sh"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^victims=\(([^)]*)\)$`).FindAllSubmatch(body, -1)
	if len(found) != 1 {
		t.Fatalf("scripts/classify-victims.sh: %d victims=(...) lines, want 1", len(found))
	}
	return strings.Fields(string(found[0][1]))
}

func victimDirs(t *testing.T) []string {
	t.Helper()
	mains, err := filepath.Glob(filepath.Join(repoRoot, "victims", "*", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(mains))
	for _, main := range mains {
		names = append(names, "victim-"+filepath.Base(filepath.Dir(main)))
	}
	return names
}

func snapshotNames(t *testing.T) []string {
	t.Helper()
	snapshots, err := filepath.Glob(filepath.Join(repoRoot, "config", "gateway", "tools", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		names = append(names, strings.TrimSuffix(filepath.Base(snapshot), ".json"))
	}
	return names
}

func classifiedUpstreams(t *testing.T) []string {
	t.Helper()
	var classes []gateway.Class
	readYAML(t, "config/gateway/classification.yaml", &classes)
	names := make([]string, 0, len(classes))
	for _, class := range classes {
		names = append(names, class.Upstream)
	}
	return names
}

func fingerprintedUpstreams(t *testing.T) []string {
	t.Helper()
	var pins []struct {
		Upstream    string `json:"upstream"`
		Tool        string `json:"tool"`
		Fingerprint string `json:"fingerprint"`
	}
	readYAML(t, "config/gateway/fingerprints.yaml", &pins)
	names := make([]string, 0, len(pins))
	for _, pin := range pins {
		names = append(names, pin.Upstream)
	}
	return names
}
