package main

import (
	"strings"
	"testing"
)

func TestComposeArgsCarryTheFileTheEnvFileAndEveryProfile(t *testing.T) {
	got := composeArgs("compose/compose.yaml", "versions.env", []string{"core", "chaos"},
		[]string{"up", "-d", "--build", "victim-fs"})
	want := "--env-file versions.env -f compose/compose.yaml --profile core --profile chaos up -d --build victim-fs"
	if strings.Join(got, " ") != want {
		t.Errorf("argv is %q,\nwant %q", strings.Join(got, " "), want)
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
