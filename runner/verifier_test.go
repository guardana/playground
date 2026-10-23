package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/dockercompose"
)

const (
	verifierScenarioFile = `schema_version: 1
id: verify-01
title: a pinned manifest that changes is reported as drift
profile: [verifier]
verifier:
  - probe: { server: victim-fs, write_pin: true }
  - probe: { server: victim-fs, pin_from: 1 }
expect:
  verifier:
    1: { exit_code: 0 }
    2:
      exit_code: 1
      findings_include: [{ rule_id: guardana.agent.mcp_server_manifest, summary_contains: "'fs.read'" }]
      findings_exclude: [guardana.mcp.cache_scope]
  effects:
    victim-fs: { calls_served: {} }
`
	verifierReport = `{"schema_version": 18, "run": {"target": {"ref": "http://victim-fs:8080/mcp"},
 "result_summary": {"rules_run": ["guardana.agent.mcp_server_manifest", "guardana.mcp.cache_scope"]}},
 "findings": [{"rule_id": "guardana.agent.mcp_server_manifest", "severity": "CRITICAL",
  "evidence": {"summary": "the declaration of 'fs.read' changed after it was approved (rug pull)"}}],
 "unverified": [], "waived": [], "errors": []}`
	verifierPin = `{"schema_version": 2, "server": "http://victim-fs:8080/mcp", "tools": {"fs.read": "sha256:00"}}`
)

func (f *fakeCompose) Stop(ctx context.Context, _ []string, service string) error {
	if err := f.called(ctx, "stop "+service); err != nil {
		return err
	}
	f.stopped = append(f.stopped, service)
	return nil
}

func (f *fakeCompose) Exec(ctx context.Context, _ []string, service string, args []string) (dockercompose.Split, error) {
	if err := f.called(ctx, "exec "+service+" "+strings.Join(args, " ")); err != nil {
		return dockercompose.Split{}, err
	}
	f.execs = append(f.execs, append([]string{service}, args...))
	if url := listenerURLOf(args); url != "" && (url == listenerURL || url == f.agentURL()) {
		if f.listener != nil {
			return f.listener(service, url), nil
		}
		if url == listenerURL {
			return dockercompose.Split{ExitCode: 1, Stderr: `calling "initialize": Post "` + url + `": dial tcp 172.30.0.4:8080: connect: connection refused`}, nil
		}
		return dockercompose.Split{ExitCode: 1, Stderr: `Post "` + url + `": dial tcp ` + f.env["LAB_ENFORCER_ADDRESS"] + `:8080: connect: network is unreachable`}, nil
	}
	if f.exec == nil {
		return dockercompose.Split{ExitCode: 1, Stderr: "the fake has no exec"}, nil
	}
	return f.exec(service, args), nil
}

// listenerURLOf is the URL a /relist command lists, empty for any other.
func listenerURLOf(args []string) string {
	if len(args) == 2 && args[0] == "/relist" {
		return args[1]
	}
	return ""
}

// agentURL is the listener at the enforcer's agent-net address in this run.
func (f *fakeCompose) agentURL() string {
	return "http://" + f.env["LAB_ENFORCER_ADDRESS"] + ":8080/mcp"
}

func (f *fakeCompose) Logs(ctx context.Context, _ []string, service string) (dockercompose.Split, error) {
	if err := f.called(ctx, "logs "+service); err != nil {
		return dockercompose.Split{}, err
	}
	if f.logs != nil {
		return f.logs(service), nil
	}
	return dockercompose.Split{Stdout: "listening for agents on " + f.env["LAB_ENFORCER_ADDRESS"] + ":8080\n"}, nil
}

func (f *fakeCompose) RunSplit(ctx context.Context, _ []string, service, entrypoint string, args []string) (dockercompose.Split, error) {
	if err := f.called(ctx, "run "+service+" "+strings.Join(args, " ")); err != nil {
		return dockercompose.Split{}, err
	}
	f.ran = append(f.ran, append([]string{service, entrypoint}, args...))
	if f.split == nil {
		return dockercompose.Split{}, errors.New("no verifier in this fake")
	}
	return f.split(service, entrypoint, args)
}

// splitCall is one RunSplit as compose would have run it.
type splitCall struct {
	service  string
	profiles []string
	env      map[string]string
	runEnv   []string
	args     []string
}

// verifierBinds are the host directories compose binds into a verifier
// service for the call the fake answers, by the target they are bound at. A
// source that is not a directory is refused, as compose refuses it.
func verifierBinds(f *fakeCompose, service string) (map[string]string, error) {
	step := f.callEnv["LAB_VERIFIER_DIR"]
	if step == "" {
		step = "LAB_VERIFIER_DIR-is-unset"
	}
	under := filepath.Join(f.env["LAB_RUN_HOST_DIR"], "verifier", step)
	binds := map[string]string{"/lab-run/in": filepath.Join(under, "in")}
	if service == "verifier-pin" {
		binds["/lab-run/out"] = filepath.Join(under, "out")
	}
	for _, source := range binds {
		if info, err := os.Stat(source); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("bind source path does not exist: %s", source)
		}
	}
	return binds, nil
}

// onHost is the host path of a container path under one of the binds.
func onHost(binds map[string]string, inContainer string) (string, bool) {
	for target, source := range binds {
		if rest, found := strings.CutPrefix(inContainer, target+"/"); found {
			return filepath.Join(source, rest), true
		}
	}
	return "", false
}

// workingVerifier answers as the verifier, the victims and the sealed network
// do: every victim's journal is written, the pin lands in the directory compose
// binds writable, and the report is printed on standard output. Like compose,
// it refuses to start the verifier when a directory it binds is not there, and
// like the verifier it exits 3 on a pin it cannot read.
func workingVerifier(f *fakeCompose) func(string, string, []string) (dockercompose.Split, error) {
	return func(service, entrypoint string, args []string) (dockercompose.Split, error) {
		binds, err := verifierBinds(f, service)
		if err != nil {
			return dockercompose.Split{}, err
		}
		runDir := f.env["LAB_RUN_HOST_DIR"]
		for _, victim := range []string{"victim-fs", "victim-crm"} {
			writeFile(filepath.Join(runDir, "journals", victim, victim+".jsonl"), "")
		}
		if entrypoint == "python" {
			return reachAnswer(args), nil
		}
		if i := slices.Index(args, "--write-mcp-pin"); i >= 0 {
			at, writable := onHost(binds, args[i+1])
			if !writable || !strings.HasPrefix(args[i+1], "/lab-run/out/") {
				return dockercompose.Split{ExitCode: 3, Stderr: "Read-only file system: " + args[i+1]}, nil
			}
			writeFile(at, verifierPin)
			return dockercompose.Split{Stdout: "Wrote 1 approved tool description(s)\n"}, nil
		}
		if i := slices.Index(args, "--mcp-pin"); i >= 0 {
			if at, bound := onHost(binds, args[i+1]); !bound || !exists(at) {
				return dockercompose.Split{ExitCode: 3, Stderr: "cannot read the pin " + args[i+1]}, nil
			}
		}
		return dockercompose.Split{ExitCode: 1, Stdout: verifierReport, Stderr: "compose: Container verifier Creating\n"}, nil
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func reachAnswer(args []string) dockercompose.Split {
	if slices.Contains(args, proxyScript) {
		return dockercompose.Split{Stdout: "probe unreachable " + proxyTarget + ": none in the environment\n"}
	}
	if slices.Contains(args, routeScript) {
		return dockercompose.Split{Stdout: "probe unreachable " + routeTarget + ": none in /proc/net/route or /proc/net/ipv6_route\n"}
	}
	target := args[len(args)-1]
	if target == outsideAddress {
		return dockercompose.Split{Stdout: "probe unreachable " + target + ": [Errno 101] Network is unreachable\n"}
	}
	return dockercompose.Split{Stdout: "probe reached " + target + "\n"}
}

func verifierUnderTest(t *testing.T) (lab, *fakeCompose, string) {
	t.Helper()
	compose := &fakeCompose{
		inProfile: []string{"verifier", "victim-crm", "victim-fs", "mailpit"},
		status: []assertion.Service{
			{Name: "victim-crm", Running: true, Detail: "running"},
			{Name: "victim-fs", Running: true, Detail: "running"},
			{Name: "mailpit", Running: true, Detail: "running"},
		},
	}
	compose.split = workingVerifier(compose)
	subject, _ := labUnderTest(t, compose)
	scenario := filepath.Join(subject.root, "scenarios/verify/verify-01.yaml")
	writeFile(scenario, verifierScenarioFile)
	pinVerifierImage(t, &subject, "1.0.0rc5")
	return subject, compose, scenario
}

func TestAVerifierScenarioRunsWithNoAgentAndNoGateway(t *testing.T) {
	subject, compose, scenario := verifierUnderTest(t)
	report, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if report.Outcome() != assertion.Pass {
		for _, result := range report.Results {
			t.Logf("%s: %s want=%q got=%q %s", result.Check, result.Outcome, result.Want, result.Got, result.Detail)
		}
		t.Fatalf("a whole verifier run reported %s", report.Outcome())
	}
	if slices.Contains(compose.broughtUp, "verifier") {
		t.Error("the one-shot verifier was brought up with the victims")
	}
	probes := verifierProbes(t, compose)
	want := []string{
		"verifier-pin probe --mcp http://victim-fs:8080/mcp --format json --plugins builtins --write-mcp-pin /lab-run/out/pin.json",
		"verifier probe --mcp http://victim-fs:8080/mcp --format json --plugins builtins --mcp-pin /lab-run/in/pin.json",
	}
	if !slices.Equal(probes, want) {
		t.Errorf("probes = %q,\nwant %q", probes, want)
	}
	runDir := filepath.Join(subject.reports, report.RunID)
	if got := compose.env["LAB_RUN_HOST_DIR"]; got != runDir {
		t.Errorf("LAB_RUN_HOST_DIR = %q, want %q", got, runDir)
	}
	for _, file := range []string{"step-1.stdout", "step-1.stderr", "step-1/out/pin.json", "step-2/in/pin.json", "step-2.json", "step-2.stderr"} {
		if !exists(filepath.Join(runDir, verifierDir, file)) {
			t.Errorf("%s was not kept", file)
		}
	}
}

// verifierProbes lists each probe the runner ran as "<service> <args>", and
// fails on any service other than the verifier's two.
func verifierProbes(t *testing.T, compose *fakeCompose) []string {
	t.Helper()
	var probes []string
	for _, ran := range compose.ran {
		if ran[0] != "verifier" && ran[0] != "verifier-pin" {
			t.Errorf("ran %v, want only the verifier", ran)
		}
		if ran[1] == "" {
			probes = append(probes, ran[0]+" "+strings.Join(ran[2:], " "))
		}
	}
	return probes
}

// Every victim the profile booted is graded, and nothing that is not a victim.
func TestAVerifierRunGradesEveryVictimItBooted(t *testing.T) {
	subject, _, scenario := verifierUnderTest(t)
	report, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var effects []string
	for _, result := range report.Results {
		if strings.HasPrefix(result.Check, "effects/") {
			effects = append(effects, result.Check+" "+result.Outcome.String())
		}
	}
	if want := []string{"effects/victim-crm pass", "effects/victim-fs pass"}; !slices.Equal(effects, want) {
		t.Errorf("effects = %q, want %q", effects, want)
	}
}

// recordSpoils each turn one record of a working verifier run red, and name
// the check that has to say so.
func recordSpoils() map[string]struct {
	spoil func(dockercompose.Split, string, []string) dockercompose.Split
	says  string
} {
	return map[string]struct {
		spoil func(dockercompose.Split, string, []string) dockercompose.Split
		says  string
	}{
		"the report only on stderr": {func(s dockercompose.Split, _ string, args []string) dockercompose.Split {
			if slices.Contains(args, "--mcp-pin") {
				s.Stdout, s.Stderr = "", s.Stdout
			}
			return s
		}, "verifier/step-2/report"},
		"a route out": {func(s dockercompose.Split, entrypoint string, _ []string) dockercompose.Split {
			if entrypoint == "python" && strings.Contains(s.Stdout, outsideAddress) {
				s.Stdout = "probe reached " + outsideAddress + "\n"
			}
			return s
		}, "verifier-reach/no-route-out"},
		"a default route beside an unreachable dial": {func(s dockercompose.Split, entrypoint string, args []string) dockercompose.Split {
			if entrypoint == "python" && slices.Contains(args, routeScript) {
				s.Stdout = "probe reached " + routeTarget + ": ipv4 via eth0\n"
			}
			return s
		}, "verifier-reach/no-default-route"},
		"a proxy in the verifier's environment": {func(s dockercompose.Split, entrypoint string, args []string) dockercompose.Split {
			if entrypoint == "python" && slices.Contains(args, proxyScript) {
				s.Stdout = "probe reached " + proxyTarget + ": HTTPS_PROXY\n"
			}
			return s
		}, "verifier-reach/no-proxy"},
		"a drift not found": {func(s dockercompose.Split, _ string, args []string) dockercompose.Split {
			if slices.Contains(args, "--mcp-pin") {
				s.ExitCode, s.Stdout = 0, strings.Replace(s.Stdout, "'fs.read' changed", "nothing changed", 1)
			}
			return s
		}, "verifier/step-2/finding/guardana.agent.mcp_server_manifest"},
	}
}

func TestAVerifierRunIsNotGreenWhenItsRecordsAreNot(t *testing.T) {
	for name, test := range recordSpoils() {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := verifierUnderTest(t)
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (dockercompose.Split, error) {
				split, err := working(service, entrypoint, args)
				return test.spoil(split, entrypoint, args), err
			}
			report, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			for _, result := range report.Results {
				if result.Check == test.says && result.Outcome == assertion.Fail {
					return
				}
			}
			t.Errorf("no failed %s among %+v", test.says, report.Results)
		})
	}
}

// The verifier reaches every victim on tool-net, so a call it made to one the
// scenario never probed is still a call, and a victim with no journal is still
// no answer.
func TestEveryVictimInTheProfileIsGradedAsServingNothing(t *testing.T) {
	for name, journal := range map[string]string{
		"a served call": `{"occurred_at":"2026-09-09T12:00:03Z","server":"victim-crm","tool":"crm.delete_customer","run_id":"${RUN_ID}","status":"served"}` + "\n",
		"no journal":    "",
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := verifierUnderTest(t)
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (dockercompose.Split, error) {
				split, err := working(service, entrypoint, args)
				crm := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "journals", "victim-crm", "victim-crm.jsonl")
				if journal == "" {
					_ = os.Remove(crm)
				} else {
					writeFile(crm, strings.ReplaceAll(journal, "${RUN_ID}", compose.env["LAB_RUN_ID"]))
				}
				return split, err
			}
			report, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			outcomes := map[string]assertion.Outcome{}
			for _, result := range report.Results {
				outcomes[result.Check] = result.Outcome
			}
			if outcomes["effects/victim-crm"] != assertion.Fail || outcomes["effects/victim-fs"] != assertion.Pass {
				t.Errorf("effects = crm %s, fs %s; want crm fail, fs pass", outcomes["effects/victim-crm"], outcomes["effects/victim-fs"])
			}
		})
	}
}
