package compose

import (
	"os"
	"reflect"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

type hardenedService struct {
	Image       string   `json:"image"`
	User        string   `json:"user"`
	ReadOnly    bool     `json:"read_only"`
	CapDrop     []string `json:"cap_drop"`
	SecurityOpt []string `json:"security_opt"`
	Tmpfs       []string `json:"tmpfs"`
	Networks    []string `json:"networks"`
	Profiles    []string `json:"profiles"`
	Volumes     []any    `json:"volumes"`
}

// runVerifierMount is the one mount the verifier gets: the run's verifier/
// directory, never the run directory whose journals it is graded from, and a
// source that is refused rather than created when the runner did not name one.
var runVerifierMount = map[string]any{
	"type":   "bind",
	"source": "${LAB_RUN_HOST_DIR:-/LAB_RUN_HOST_DIR-is-unset}/verifier",
	"target": "/lab-run",
	"bind":   map[string]any{"create_host_path": false},
}

// The verifier connects to servers a scenario names and those servers answer
// it, so it gets what every lab service gets and nothing it does not need: one
// sealed network, no privilege, and the one directory of a run it writes into.
func TestTheVerifierIsHardenedAndSeesOneRun(t *testing.T) {
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
	verifier, declared := parsed.Services["verifier"]
	if !declared {
		t.Fatal("compose.yaml declares no verifier")
	}
	for what, held := range map[string]bool{
		"its image named by the pins": verifier.Image == "${VERIFIER_IMAGE}:${VERIFIER_VERSION}",
		"uid 65532":                   verifier.User == "65532:65532",
		"a read-only root":            verifier.ReadOnly,
		"no capability":               slices.Equal(verifier.CapDrop, []string{"ALL"}),
		"no new privileges":           slices.Contains(verifier.SecurityOpt, "no-new-privileges:true"),
		"a tmpfs /tmp":                slices.Equal(verifier.Tmpfs, []string{"/tmp"}),
		"tool-net alone":              slices.Equal(verifier.Networks, []string{"tool-net"}),
		"its own profile alone":       slices.Equal(verifier.Profiles, []string{"verifier"}),
		"the run's verifier directory alone": len(verifier.Volumes) == 1 &&
			reflect.DeepEqual(verifier.Volumes[0], runVerifierMount),
	} {
		if !held {
			t.Errorf("the verifier does not have %s: %+v", what, verifier)
		}
	}
}
