package compose

import (
	"os"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

// A victim is the thing the agent is turned against, and victim-shell hands
// out a shell. Each one is held to the hardening by name, so a victim that
// stops merging the anchor, or overrides one key of it, fails here and not in
// a review.
func TestEveryVictimIsHardenedOnToolNetAlone(t *testing.T) {
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]hardenedService `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	for _, name := range labspec.Victims() {
		victim, declared := parsed.Services[name]
		if !declared {
			t.Errorf("compose.yaml declares no %s", name)
			continue
		}
		for what, held := range map[string]bool{
			"every capability dropped": slices.Contains(victim.CapDrop, "ALL"),
			"no capability added":      len(victim.CapAdd) == 0,
			"a read-only root":         victim.ReadOnly,
			"no new privileges":        slices.Contains(victim.SecurityOpt, "no-new-privileges:true"),
			"no privileged mode":       !victim.Privileged,
			"no published port":        len(victim.Ports) == 0,
			"tool-net alone":           slices.Equal(victim.Networks, []string{"tool-net"}),
		} {
			if !held {
				t.Errorf("%s does not have %s: %+v", name, what, victim)
			}
		}
	}
}
