package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/guardana/playground/internal/assertion"
)

// Every image the run uses is built before the plane starts: an image built
// while the plane runs spends minutes of the bundle's freshness, and the agent
// is first run by the probes, after the plane is up.
func TestBootBuildsEveryImageBeforeAnythingStarts(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() != assertion.Pass {
		t.Fatalf("the run is %s", graded.Outcome())
	}
	built := slices.Index(compose.calls, "build")
	started := slices.IndexFunc(compose.calls, func(call string) bool {
		return strings.HasPrefix(call, "up ") || strings.HasPrefix(call, "run ")
	})
	if built < 0 || started < 0 || built > started {
		t.Errorf("docker calls in order: %v; want one build before the first up or run", compose.calls)
	}
	if !slices.Contains(compose.built, "agent") || !slices.Contains(compose.built, "enforcer") {
		t.Errorf("the build covered profiles %v; want the scenario's and the agent's", compose.built)
	}
}

func TestAFailedBuildBringsNothingUp(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.buildErr = errors.New("docker compose build: exit status 1: go: module lookup disabled")
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() == assertion.Pass {
		t.Fatal("a run whose images did not build passed")
	}
	if len(compose.broughtUp) != 0 {
		t.Errorf("a run whose images did not build brought up %v", compose.broughtUp)
	}
	found := results(graded)["boot/victim-fs"]
	if found.Outcome != assertion.Fail || !strings.Contains(found.Detail, "module lookup disabled") {
		t.Errorf("boot/victim-fs is %+v, want failed naming the build", found)
	}
}

// Boot built the images; a later build would spend the plane's time again, and
// a run that built nothing up front would start whatever an older checkout left.
func TestNothingButTheBootBuilds(t *testing.T) {
	up := strings.Join(upArgs([]string{"victim-fs"}), " ")
	if want := "up -d --wait victim-fs"; up != want {
		t.Errorf("up is %q, want %q", up, want)
	}
	run := strings.Join(runOnceArgs("scripted-agent", []string{"-probe", "x:1"}), " ")
	if want := "run --rm --no-TTY scripted-agent -probe x:1"; run != want {
		t.Errorf("run is %q, want %q", run, want)
	}
}

// The boot's build names the agent's profile by the constant; compose has to
// put the agent there and nowhere else, or the build skips the agent's image.
func TestTheAgentIsBuiltUnderItsOwnProfile(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", composeFile))
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Services map[string]struct {
			Profiles []string `json:"profiles"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := parsed.Services[agentService].Profiles; !slices.Equal(got, []string{agentProfile}) {
		t.Errorf("%s is in profiles %v, want [%s]", agentService, got, agentProfile)
	}
}

// A profile that did not come up says why in the boot record, not only in the
// runner's own output: two runs that drew one agent-net subnet fail here.
func TestAFailedUpNamesItsCauseInTheBootRecord(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.upErr = errors.New("docker compose up: exit status 1: networks have overlapping IPv4")
	compose.status = []assertion.Service{{Name: "enforcer", Detail: "no container: compose reported nothing"}}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := results(graded)["boot/enforcer"]
	if found.Outcome != assertion.Fail || !strings.Contains(found.Detail, "overlapping IPv4") {
		t.Errorf("boot/enforcer is %s: %s", found.Outcome, found.Detail)
	}
	record, err := os.ReadFile(filepath.Join(subject.reports, graded.RunID, "boot.json"))
	if err != nil || !strings.Contains(string(record), "overlapping IPv4") {
		t.Errorf("boot.json does not say why (%v): %s", err, record)
	}
}
