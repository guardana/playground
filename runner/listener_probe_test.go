package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const listenerURL = "http://enforcer:8080/mcp"

// A service on one of the enforcer's other networks asks its agent listener
// for a session: the victim the trajectory calls first, and the decision point
// double when its profile is up. The agent is not one of them; it has its own
// probe.
func TestTheListenerIsProbedFromTheEnforcersOtherNetworks(t *testing.T) {
	withDouble := strings.NewReplacer("profile: [core, enforcer]", "profile: [core, enforcer, pdp]",
		"policy: config/policies/flow-01.json }", "policy: config/policies/flow-01.json, pdp_script: veto.yaml }")
	for name, test := range map[string]struct {
		scenario string
		from     []string
	}{
		"core":                     {scenarioFile, []string{"victim-fs"}},
		"with the decision double": {withDouble.Replace(scenarioFile), []string{"victim-fs", "pdp-double"}},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := enforcerLab(t)
			writeFile(scenario, test.scenario)
			writeFile(filepath.Join(subject.root, "config/pdp/veto.yaml"), "rules: []\n")
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			var probed []string
			for _, exec := range compose.execs {
				if slices.Equal(exec[1:], []string{"/relist", listenerURL}) {
					probed = append(probed, exec[0])
				}
			}
			if !slices.Equal(probed, test.from) {
				t.Errorf("the listener was probed from %v, want %v", probed, test.from)
			}
			found := results(graded)
			record, err := os.ReadFile(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "probes.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, from := range test.from {
				result := found["network-isolation/listener-closed-to/"+from]
				if result.Outcome != assertion.Pass || !strings.HasSuffix(result.Source, "probes.log") {
					t.Errorf("the probe from %s is %s: %+v", from, result.Outcome, result)
				}
				if !strings.Contains(string(record), "listener enforcer:8080 from "+from+" ran=true reached=false ") {
					t.Errorf("probes.log does not record the probe from %s:\n%s", from, record)
				}
			}
		})
	}
}

// The deliberate failure the check exists for: the listener answers a victim.
func TestAVictimThatListsTheEnforcersToolsFailsTheRun(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.listener = func(string, string) Split {
		return Split{Stdout: `{"name":"fs.read","description":"Read a file."}` + "\n"}
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	result := results(graded)["network-isolation/listener-closed-to/victim-fs"]
	if result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "fs.read") {
		t.Errorf("a victim that listed the enforcer's tools was %s: %+v", result.Outcome, result)
	}
	if graded.Outcome() != assertion.Fail {
		t.Errorf("the run is %s", graded.Outcome())
	}
}
