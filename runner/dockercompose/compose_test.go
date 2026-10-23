package dockercompose

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestComposeArgsCarryTheFileTheEnvFileAndEveryProfile(t *testing.T) {
	got := composeArgs("compose/compose.yaml", "versions.env", []string{"core", "chaos"},
		[]string{"up", "-d", "--build", "victim-fs"})
	want := "--env-file versions.env -f compose/compose.yaml --profile core --profile chaos up -d --build victim-fs"
	if strings.Join(got, " ") != want {
		t.Errorf("argv is %q,\nwant %q", strings.Join(got, " "), want)
	}
}

// A split run builds nothing, so the verifier image the runner checked is the
// one that runs.
func TestASplitRunNamesItsEntrypointBeforeTheService(t *testing.T) {
	got := strings.Join(runSplitArgs("verifier", "python", []string{"-c", "x"}, nil), " ")
	if want := "run --rm --no-TTY --entrypoint python verifier -c x"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
	got = strings.Join(runSplitArgs("verifier", "", []string{"probe"}, nil), " ")
	if want := "run --rm --no-TTY verifier probe"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
}

// A variable reaches the container by its name alone: compose takes the value
// from its own environment, so a token never appears on the command line.
func TestARunHandsAVariableOverByNameBeforeTheService(t *testing.T) {
	got := strings.Join(runSplitArgs("verifier", "", []string{"probe"}, []string{"LAB_MCP_TOKEN"}), " ")
	if want := "run --rm --no-TTY -e LAB_MCP_TOKEN verifier probe"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
}

func TestParseStatusReadsBothShapesComposeWrites(t *testing.T) {
	lines := `{"Name":"lab-victim-fs-1","Service":"victim-fs","State":"running","Health":"healthy"}
{"Name":"lab-enforcer-1","Service":"enforcer","State":"exited","ExitCode":1,"Health":""}
`
	array := `[
  {"Name":"lab-victim-fs-1","Service":"victim-fs","State":"running","Health":"healthy"},
  {"Name":"lab-enforcer-1","Service":"enforcer","State":"exited","ExitCode":1,"Health":""}
]`

	for name, output := range map[string]string{"one object per line": lines, "one array": array} {
		t.Run(name, func(t *testing.T) {
			services, err := parseStatus(output, []string{"victim-fs", "enforcer", "victim-mail"})
			if err != nil {
				t.Fatalf("parseStatus: %v", err)
			}
			if len(services) != 3 {
				t.Fatalf("got %d services, want one per service the runner asked for", len(services))
			}
			if !services[0].Running {
				t.Errorf("a running service was read as not running: %+v", services[0])
			}
			if services[1].Running {
				t.Errorf("an exited service was read as running: %+v", services[1])
			}
			if !strings.Contains(services[1].Detail, "exited") {
				t.Errorf("the detail does not say what happened: %+v", services[1])
			}
			// A service compose never reported on is not running, and saying
			// nothing about it would leave the run graded on a silence.
			if services[2].Running || !strings.Contains(services[2].Detail, "no container") {
				t.Errorf("a service with no container was read as %+v", services[2])
			}
		})
	}
}

func TestParseStatusTreatsAnUnhealthyContainerAsNotRunning(t *testing.T) {
	output := `{"Service":"victim-fs","State":"running","Health":"starting"}`

	services, err := parseStatus(output, []string{"victim-fs"})
	if err != nil {
		t.Fatalf("parseStatus: %v", err)
	}
	if services[0].Running {
		t.Errorf("a container that has not passed its health check was read as ready: %+v", services[0])
	}
	if !strings.Contains(services[0].Detail, "starting") {
		t.Errorf("the detail does not carry the health: %+v", services[0])
	}
}

func TestParseStatusRefusesOutputItCannotRead(t *testing.T) {
	if _, err := parseStatus("this is not JSON", []string{"victim-fs"}); err == nil {
		t.Error("output that is not JSON was read as a boot record")
	}
}

// A container printing without end is stopped at the bound and the run is an
// error, never a record cut at an arbitrary byte.
func TestASplitRunRefusesAStreamPastItsBound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := splitWithin(exec.CommandContext(ctx, "sh", "-c", "yes"), 1<<10); err == nil {
		t.Error("an endless standard output was kept")
	}
	if _, err := splitWithin(exec.CommandContext(ctx, "sh", "-c", "yes >&2"), 1<<10); err == nil {
		t.Error("an endless standard error was kept")
	}
	split, err := splitWithin(exec.CommandContext(ctx, "sh", "-c", "printf %01024d 0"), 1<<10)
	if err != nil || len(split.Stdout) != 1<<10 {
		t.Errorf("output at the bound gave %d bytes, %v", len(split.Stdout), err)
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

// A variable added for one call keeps every variable the run was given: a
// step that lost COMPOSE_PROJECT_NAME would run in another project.
func TestAVariableAddedForOneCallKeepsTheRunsOwn(t *testing.T) {
	run := CLI{}.WithEnv(map[string]string{"COMPOSE_PROJECT_NAME": "lab-run1", "LAB_RUN_HOST_DIR": "/r"})
	step := run.WithEnv(map[string]string{"LAB_VERIFIER_DIR": "step-1"}).(CLI)
	got := map[string]string{}
	for _, entry := range step.Env {
		name, value, _ := strings.Cut(entry, "=")
		got[name] = value
	}
	for name, want := range map[string]string{"COMPOSE_PROJECT_NAME": "lab-run1", "LAB_RUN_HOST_DIR": "/r", "LAB_VERIFIER_DIR": "step-1"} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	for _, entry := range run.(CLI).Env {
		if strings.HasPrefix(entry, "LAB_VERIFIER_DIR=") {
			t.Error("a variable added for one call reached the run's own compose")
		}
	}
}
