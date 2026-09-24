package labcheck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/gateway"
)

// Every tool a victim lists is classified by the lab and pinned to the
// fingerprint the enforcer printed for it, and nothing is classified or pinned
// that no victim lists.
func TestEveryListedToolIsClassifiedAndPinned(t *testing.T) {
	listed := listedTools(t)
	if len(listed) == 0 {
		t.Fatal("no victim listing snapshot under config/gateway/tools")
	}
	var classes []gateway.Class
	var prints []gateway.Fingerprint
	readYAML(t, "config/gateway/classification.yaml", &classes)
	readYAML(t, "config/gateway/fingerprints.yaml", &prints)
	for _, name := range listed {
		upstream, tool, _ := strings.Cut(name, "/")
		classified := slices.ContainsFunc(classes, func(c gateway.Class) bool { return c.Upstream == upstream && c.Tool == tool })
		if !classified {
			t.Errorf("%s is listed and config/gateway/classification.yaml does not classify it", name)
		}
	}
	for _, class := range classes {
		if !slices.Contains(listed, class.Upstream+"/"+class.Tool) {
			t.Errorf("%s/%s is classified and no victim lists it", class.Upstream, class.Tool)
		}
	}
	for _, printed := range prints {
		if !slices.Contains(listed, printed.Upstream+"/"+printed.Tool) {
			t.Errorf("%s/%s is fingerprinted and no victim lists it", printed.Upstream, printed.Tool)
		}
	}
	if _, err := gateway.Overrides(classes, prints, nil); err != nil {
		t.Errorf("the classification does not pin to the fingerprints: %v", err)
	}
}

func listedTools(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot, "config", "gateway", "tools", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, path := range paths {
		body, err := os.ReadFile(path) // #nosec G304 -- the lab's own snapshots.
		if err != nil {
			t.Fatal(err)
		}
		var tools []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(body, &tools); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		upstream := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, tool := range tools {
			names = append(names, upstream+"/"+tool.Name)
		}
	}
	return names
}

func readYAML(t *testing.T, relative string, into any) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, relative)) // #nosec G304 -- the lab's own files.
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.UnmarshalStrict(body, into); err != nil {
		t.Fatalf("%s: %v", relative, err)
	}
}

// A scenario the enforcer decides names a configuration part and a policy that
// exist, and every configuration part and policy is some scenario's.
func TestEveryGatewayFileExistsAndIsUsed(t *testing.T) {
	used := map[string]bool{}
	for _, path := range filesUnder(t, "scenarios") {
		scenario, err := labspec.LoadScenario(path)
		if err != nil || !scenario.UsesEnforcer() {
			continue
		}
		for _, named := range []string{scenario.Gateway.Config, scenario.Gateway.Policy} {
			used[named] = true
			if _, err := os.Stat(filepath.Join(repoRoot, named)); err != nil {
				t.Errorf("%s names %s, which does not exist", relative(path), named)
			}
		}
	}
	if len(used) == 0 {
		t.Fatal("no scenario is decided by the enforcer; this guard inspected nothing")
	}
	for _, directory := range []string{labspec.GatewayConfigDir, labspec.PolicyDir} {
		for _, name := range unused(t, directory, used) {
			t.Errorf("%s is no scenario's", name)
		}
	}
}

// unused lists the files of directory no scenario names, the classification's
// own policy aside.
func unused(t *testing.T, directory string, used map[string]bool) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repoRoot, directory))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		name := directory + entry.Name()
		if !entry.IsDir() && !used[name] && name != labspec.PolicyDir+"classify.json" {
			names = append(names, name)
		}
	}
	return names
}
