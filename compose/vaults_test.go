package compose

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/labspec"
)

type gatedService struct {
	hardenedService
	Build struct {
		Args map[string]string `json:"args"`
	} `json:"build"`
	Environment map[string]string `json:"environment"`
	Healthcheck struct {
		Test []string `json:"test"`
	} `json:"healthcheck"`
	DependsOn map[string]json.RawMessage `json:"depends_on"`
}

func readGated(t *testing.T) map[string]gatedService {
	t.Helper()
	body, err := os.ReadFile("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]gatedService `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("compose.yaml: %v", err)
	}
	return parsed.Services
}

// A token-gated victim boots with the verifier alone: the enforcer lists every
// upstream's tools without a token, so in a core run a gated upstream would
// refuse it. It is given the run's token by the variable the runner sets, and
// an unset one reaches it empty, so its own refusal to start is what a run
// without a token meets.
func TestEveryGatedVictimBootsWithTheVerifierAloneAndTheRunsToken(t *testing.T) {
	services := readGated(t)
	for _, name := range labspec.VerifierVictims() {
		victim, declared := services[name]
		if !declared {
			t.Errorf("compose.yaml declares no %s", name)
			continue
		}
		for what, held := range map[string]bool{
			"its image named by the lab prefix": victim.Image == "${LAB_IMAGE_PREFIX}-"+name,
			"its own package built":             victim.Build.Args["TARGET"] == "./victims/"+strings.TrimPrefix(name, "victim-"),
			"uid 65532":                         victim.User == "65532:65532",
			"a tmpfs /tmp":                      slices.Equal(victim.Tmpfs, []string{"/tmp"}),
			"the verifier profile alone":        slices.Equal(victim.Profiles, []string{"verifier"}),
			"the run's token, or none":          victim.Environment[labspec.MCPTokenVariable] == "${"+labspec.MCPTokenVariable+":-}",
			"the listener its resource names":   victim.Environment["LAB_LISTEN"] == ":8080",
			"a health check on its own listener": slices.Equal(victim.Healthcheck.Test,
				[]string{"CMD", "/healthprobe", "http://127.0.0.1:8080/healthz"}),
		} {
			if !held {
				t.Errorf("%s does not have %s: %+v", name, what, victim)
			}
		}
	}
}

// The run's token reaches the gated victims and no other service through the
// compose file, and nothing waits on a gated victim, so no profile but the
// verifier's needs one up.
func TestTheTokenAndTheGatedVictimsStayInTheVerifierProfile(t *testing.T) {
	gated := labspec.VerifierVictims()
	checked := 0
	for name, service := range readGated(t) {
		if _, named := service.Environment[labspec.MCPTokenVariable]; named && !slices.Contains(gated, name) {
			t.Errorf("%s is given %s", name, labspec.MCPTokenVariable)
		}
		for waited := range service.DependsOn {
			checked++
			if slices.Contains(gated, waited) {
				t.Errorf("%s waits on %s", name, waited)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no service waits on any other; this test inspected nothing")
	}
}
