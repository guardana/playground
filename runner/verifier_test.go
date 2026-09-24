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
	verifierReport = `{"schema_version": 6, "run": {"target": {"ref": "http://victim-fs:8080/mcp"},
 "result_summary": {"rules_run": ["guardana.agent.mcp_server_manifest", "guardana.mcp.cache_scope"]}},
 "findings": [{"rule_id": "guardana.agent.mcp_server_manifest", "severity": "CRITICAL",
  "evidence": {"summary": "the declaration of 'fs.read' changed after it was approved (rug pull)"}}],
 "unverified": [], "errors": []}`
	verifierPin = `{"schema_version": 2, "server": "http://victim-fs:8080/mcp", "tools": {"fs.read": "sha256:00"}}`
)

func (f *fakeCompose) Stop(ctx context.Context, _ []string, service string) error {
	if err := f.called(ctx, "stop "+service); err != nil {
		return err
	}
	f.stopped = append(f.stopped, service)
	return nil
}

func (f *fakeCompose) Exec(ctx context.Context, _ []string, service string, args []string) (Split, error) {
	if err := f.called(ctx, "exec "+service+" "+strings.Join(args, " ")); err != nil {
		return Split{}, err
	}
	f.execs = append(f.execs, append([]string{service}, args...))
	if f.exec == nil {
		return Split{ExitCode: 1, Stderr: "the fake has no exec"}, nil
	}
	return f.exec(service, args), nil
}

func (f *fakeCompose) RunSplit(ctx context.Context, _ []string, service, entrypoint string, args []string) (Split, error) {
	if err := f.called(ctx, "run "+service+" "+strings.Join(args, " ")); err != nil {
		return Split{}, err
	}
	f.ran = append(f.ran, append([]string{service, entrypoint}, args...))
	if f.split == nil {
		return Split{}, errors.New("no verifier in this fake")
	}
	return f.split(service, entrypoint, args)
}

// workingVerifier answers as the verifier, the victims and the sealed network
// do: every victim's journal is written, the pin lands in the directory compose
// mounts, and the report is printed on standard output. Like compose, it
// refuses to start the verifier when the directory it mounts is not there.
func workingVerifier(f *fakeCompose) func(string, string, []string) (Split, error) {
	return func(_, entrypoint string, args []string) (Split, error) {
		runDir := f.env["LAB_RUN_HOST_DIR"]
		mounted := filepath.Join(runDir, verifierDir)
		if _, err := os.Stat(mounted); err != nil {
			return Split{}, fmt.Errorf("bind source path does not exist: %w", err)
		}
		for _, victim := range []string{"victim-fs", "victim-crm"} {
			writeFile(filepath.Join(runDir, "journals", victim+".jsonl"), "")
		}
		if entrypoint == "python" && slices.Contains(args, routeScript) {
			return Split{Stdout: "probe unreachable " + routeTarget + ": none in /proc/net/route or /proc/net/ipv6_route\n"}, nil
		}
		if entrypoint == "python" {
			target := args[len(args)-1]
			if target == outsideAddress {
				return Split{Stdout: "probe unreachable " + target + ": [Errno 101] Network is unreachable\n"}, nil
			}
			return Split{Stdout: "probe reached " + target + "\n"}, nil
		}
		if i := slices.Index(args, "--write-mcp-pin"); i >= 0 {
			writeFile(filepath.Join(mounted, strings.TrimPrefix(args[i+1], verifierMount)), verifierPin)
			return Split{Stdout: "Wrote 1 approved tool description(s)\n"}, nil
		}
		return Split{ExitCode: 1, Stdout: verifierReport, Stderr: "compose: Container verifier Creating\n"}, nil
	}
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
	pinVerifierImage(t, &subject, "0.26.1")
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
	var probes []string
	for _, ran := range compose.ran {
		if ran[0] != verifierService {
			t.Errorf("ran %v, want only the verifier", ran)
		}
		if ran[1] == "" {
			probes = append(probes, strings.Join(ran[2:], " "))
		}
	}
	want := []string{
		"probe --mcp http://victim-fs:8080/mcp --format json --write-mcp-pin /lab-run/pin-1.json",
		"probe --mcp http://victim-fs:8080/mcp --format json --mcp-pin /lab-run/pin-1.json",
	}
	if !slices.Equal(probes, want) {
		t.Errorf("probes = %q,\nwant %q", probes, want)
	}
	runDir := filepath.Join(subject.reports, report.RunID)
	if got := compose.env["LAB_RUN_HOST_DIR"]; got != runDir {
		t.Errorf("LAB_RUN_HOST_DIR = %q, want %q", got, runDir)
	}
	for _, file := range []string{"step-1.stdout", "step-1.stderr", "pin-1.json", "step-2.json", "step-2.stderr"} {
		if _, err := os.Stat(filepath.Join(runDir, verifierDir, file)); err != nil {
			t.Errorf("%s was not kept: %v", file, err)
		}
	}
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

func TestAVerifierRunIsNotGreenWhenItsRecordsAreNot(t *testing.T) {
	for name, test := range map[string]struct {
		spoil func(Split, string, []string) Split
		says  string
	}{
		"the report only on stderr": {func(s Split, _ string, args []string) Split {
			if slices.Contains(args, "--mcp-pin") {
				s.Stdout, s.Stderr = "", s.Stdout
			}
			return s
		}, "verifier/step-2/report"},
		"a route out": {func(s Split, entrypoint string, _ []string) Split {
			if entrypoint == "python" && strings.Contains(s.Stdout, outsideAddress) {
				s.Stdout = "probe reached " + outsideAddress + "\n"
			}
			return s
		}, "verifier-reach/no-route-out"},
		"a default route beside an unreachable dial": {func(s Split, entrypoint string, args []string) Split {
			if entrypoint == "python" && slices.Contains(args, routeScript) {
				s.Stdout = "probe reached " + routeTarget + ": ipv4 via eth0\n"
			}
			return s
		}, "verifier-reach/no-default-route"},
		"a drift not found": {func(s Split, _ string, args []string) Split {
			if slices.Contains(args, "--mcp-pin") {
				s.ExitCode, s.Stdout = 0, strings.Replace(s.Stdout, "'fs.read' changed", "nothing changed", 1)
			}
			return s
		}, "verifier/step-2/finding/guardana.agent.mcp_server_manifest"},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := verifierUnderTest(t)
			working := compose.split
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
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
			compose.split = func(service, entrypoint string, args []string) (Split, error) {
				split, err := working(service, entrypoint, args)
				crm := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "journals", "victim-crm.jsonl")
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
