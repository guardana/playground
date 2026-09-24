package main

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

// A one-shot service is built on every run: its image name is fixed, so an
// image built from an older checkout would otherwise be the one that runs.
func TestAOneShotRunBuildsItsImageFirst(t *testing.T) {
	got := strings.Join(runOnceArgs("scripted-agent", []string{"-probe", "x:1"}), " ")
	if want := "run --rm --no-TTY --build scripted-agent -probe x:1"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
}

// A split run is built by a separate command first: compose prints build
// progress on standard output, which a split run keeps as a record.
func TestASplitRunNamesItsEntrypointBeforeTheService(t *testing.T) {
	got := strings.Join(runSplitArgs("verifier", "python", []string{"-c", "x"}), " ")
	if want := "run --rm --no-TTY --entrypoint python verifier -c x"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
	got = strings.Join(runSplitArgs("verifier", "", []string{"probe"}), " ")
	if want := "run --rm --no-TTY verifier probe"; got != want {
		t.Errorf("argv is %q, want %q", got, want)
	}
}

func TestParseStatusReadsBothShapesComposeWrites(t *testing.T) {
	lines := `{"Name":"lab-victim-fs-1","Service":"victim-fs","State":"running","Health":"healthy"}
{"Name":"lab-stub-gateway-1","Service":"stub-gateway","State":"exited","ExitCode":1,"Health":""}
`
	array := `[
  {"Name":"lab-victim-fs-1","Service":"victim-fs","State":"running","Health":"healthy"},
  {"Name":"lab-stub-gateway-1","Service":"stub-gateway","State":"exited","ExitCode":1,"Health":""}
]`

	for name, output := range map[string]string{"one object per line": lines, "one array": array} {
		t.Run(name, func(t *testing.T) {
			services, err := parseStatus(output, []string{"victim-fs", "stub-gateway", "victim-mail"})
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

func TestReadProbeTellsNoRouteApartFromNotHavingRun(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		ran     bool
		reached bool
	}{
		{"reached", "probe reached stub-gateway:8080\n", true, true},
		{"no route", "probe unreachable victim-fs:8080: lookup victim-fs: no such host\n", true, false},
		{"never ran", "Error response from daemon: no such image\n", false, false},
		{"said nothing at all", "", false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := readProbe("victim-fs:8080", test.output)
			if got.Ran != test.ran || got.Reached != test.reached {
				t.Errorf("read %+v, want ran=%t reached=%t", got, test.ran, test.reached)
			}
			if got.Detail == "" {
				t.Errorf("the probe carries no reason: %+v", got)
			}
		})
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
