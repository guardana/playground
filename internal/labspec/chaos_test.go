package labspec_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/labspec"
)

const chaosLines = `chaos:
  - toxic: { victim: victim-fs, type: latency, latency: 1500ms }
  - toxic: { victim: victim-web, type: hang }
  - collector: down
  - relist: victim-fs
`

func chaosScenario() string {
	return strings.Replace(strings.Replace(heldScenario, "profile: [core, enforcer]", "profile: [core, enforcer, chaos]", 1),
		"expect:\n", chaosLines+"expect:\n", 1)
}

func TestAChaosScenarioLoadsWithEveryFault(t *testing.T) {
	scenario, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", chaosScenario()))
	if err != nil {
		t.Fatalf("LoadScenario: %v", err)
	}
	if len(scenario.Chaos) != 4 {
		t.Fatalf("loaded %d faults, want 4", len(scenario.Chaos))
	}
	latency := scenario.Chaos[0].Toxic
	if latency == nil || latency.Victim != "victim-fs" || time.Duration(latency.Latency) != 1500*time.Millisecond {
		t.Errorf("the first fault is %+v", latency)
	}
	if got := scenario.Proxied(); strings.Join(got, ",") != "victim-fs,victim-web" {
		t.Errorf("proxied = %v, want victim-fs and victim-web", got)
	}
	if scenario.Chaos[2].Collector != "down" || scenario.Chaos[3].Relist != "victim-fs" {
		t.Errorf("the last two faults are %+v and %+v", scenario.Chaos[2], scenario.Chaos[3])
	}
}

func TestAChaosFaultThatSaysNothingExactIsRefused(t *testing.T) {
	good := chaosScenario()
	for name, body := range map[string]string{
		"no chaos profile":        strings.Replace(good, "profile: [core, enforcer, chaos]", "profile: [core, enforcer]", 1),
		"a chaos profile alone":   strings.Replace(good, chaosLines, "chaos:\n  - collector: down\n", 1),
		"an unknown fault key":    strings.Replace(good, "  - collector: down\n", "  - pdp: down\n", 1),
		"two faults in one entry": strings.Replace(good, "  - collector: down\n", "  - { collector: down, relist: victim-db }\n", 1),
		"an empty entry":          strings.Replace(good, "  - collector: down\n", "  - {}\n", 1),
		"a collector not down":    strings.Replace(good, "collector: down", "collector: slow", 1),
		"the collector twice":     strings.Replace(good, "  - collector: down\n", "  - collector: down\n  - collector: down\n", 1),
		"two toxics on a victim":  strings.Replace(good, "victim: victim-web", "victim: victim-fs", 1),
		"a toxic on no victim":    strings.Replace(good, "victim: victim-web", "victim: collector", 1),
		"an unknown toxic":        strings.Replace(good, "type: hang", "type: reset", 1),
		"a latency of nothing":    strings.Replace(good, "latency: 1500ms", "latency: 0s", 1),
		"a latency under 1ms":     strings.Replace(good, "latency: 1500ms", "latency: 900us", 1),
		"a fraction of a ms":      strings.Replace(good, "latency: 1500ms", "latency: 1500.5ms", 1),
		"a latency past the wait": strings.Replace(good, "latency: 1500ms", "latency: 11m", 1),
		"no latency":              strings.Replace(good, ", latency: 1500ms", "", 1),
		"a hang with a latency":   strings.Replace(good, "type: hang", "type: hang, latency: 1s", 1),
		"a relist of no victim":   strings.Replace(good, "relist: victim-fs", "relist: enforcer", 1),
		"a relist twice":          strings.Replace(good, "  - relist: victim-fs\n", "  - relist: victim-fs\n  - relist: victim-fs\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if body == good {
				t.Fatal("the mutation did not apply")
			}
			if _, err := labspec.LoadScenario(writeFile(t, "case-01-allow-read.yaml", body)); !errors.Is(err, labspec.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestAVerifierScenarioTakesNoChaos(t *testing.T) {
	body := strings.Replace(verifierScenario, "expect:\n", "chaos:\n  - relist: victim-fs\nexpect:\n", 1)
	if _, err := loadVerifier(t, body); !errors.Is(err, labspec.ErrInvalid) || !strings.Contains(err.Error(), "chaos") {
		t.Fatalf("err = %v, want chaos refused", err)
	}
}

func TestOnErrorTakesContinueAlone(t *testing.T) {
	marked := strings.Replace(goodTrajectory, "  - call:\n      server: victim-fs", "  - on_error: continue\n    call:\n      server: victim-fs", 1)
	loaded, err := labspec.LoadTrajectory(writeFile(t, "flow.yaml", marked))
	if err != nil {
		t.Fatalf("LoadTrajectory: %v", err)
	}
	if loaded.Steps[1].OnError != labspec.OnErrorContinue || loaded.Steps[0].OnError != "" {
		t.Errorf("on_error = %q and %q", loaded.Steps[0].OnError, loaded.Steps[1].OnError)
	}
	stops := strings.Replace(marked, "on_error: continue", "on_error: ignore", 1)
	if _, err := labspec.LoadTrajectory(writeFile(t, "flow.yaml", stops)); !errors.Is(err, labspec.ErrInvalid) {
		t.Fatalf("on_error: ignore loaded: %v", err)
	}
}
