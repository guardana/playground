package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
)

const (
	scenarioFile = `schema_version: 1
id: flow-01
title: a read the policy permits is served and recorded
profile: [core, enforcer]
enforcement_mode: enforce
trajectory: trajectories/flow-01.yaml
gateway: { config: config/gateway/scenarios/flow-01.yaml, policy: config/policies/flow-01.json }
expect:
  decisions:
    1: { verdict: ALLOW, reason_codes_include: [RULE_ALLOW] }
  effects:
    victim-fs: { calls_served: { fs.read: 1 } }
  evidence:
    chain_complete: true
    policy_digest_present: true
    content_captured: false
`
	trajectoryFile = `schema_version: 1
agent: { id: support-agent, framework: scripted, model_ref: none }
principal: { id: user_123, type: human, tenant_id: tenant_a }
session: { environment: development }
steps:
  - call:
      server: victim-fs
      tool: fs.read
      args: { path: "/data/private/customers.csv" }
`
	// evidenceFile is the trail as the enforcer at its pin writes it: the mode
	// on every event, the action digest on the decision, the executed digest
	// on the completion, and no run id anywhere.
	evidenceFile = `{"eventId":"e1","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r1","projectId":"project-1","tenantId":"tenant-1","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","occurredAt":"2026-09-09T12:00:00Z","proposed":{"requestId":"r1","action":{"name":"fs.read","protocol":"mcp"}}}
{"eventId":"e2","kind":"EVENT_KIND_POLICY_DECIDED","requestId":"r1","projectId":"project-1","tenantId":"tenant-1","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","occurredAt":"2026-09-09T12:00:01Z","prevEventId":"e1","decision":{"requestId":"r1","actionDigest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","verdict":"VERDICT_ALLOW","reasonCodes":["RULE_ALLOW"],"policyBundleDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}}
{"eventId":"e3","kind":"EVENT_KIND_ACTION_STARTED","requestId":"r1","projectId":"project-1","tenantId":"tenant-1","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","occurredAt":"2026-09-09T12:00:02Z","prevEventId":"e2"}
{"eventId":"e4","kind":"EVENT_KIND_ACTION_COMPLETED","requestId":"r1","projectId":"project-1","tenantId":"tenant-1","enforcementMode":"ENFORCEMENT_MODE_ENFORCE","occurredAt":"2026-09-09T12:00:03Z","prevEventId":"e3","result":{"requestId":"r1","executedActionDigest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}}
`
	// ${RUN_ID} is what the fake victims stamp their journals with, the way the
	// real ones stamp LAB_RUN_ID.
	journalFile = `{"occurred_at":"2026-09-09T12:00:03Z","server":"victim-fs","tool":"fs.read","run_id":"${RUN_ID}","status":"served"}
`
)

// fakeCompose stands in for docker. It answers from what a test set on it and,
// when the replay runs, writes the records the services would have written, so
// a whole run can be graded without a container.
type fakeCompose struct {
	reports   string
	inProfile []string
	status    []assertion.Service
	buildErr  error
	built     []string
	upErr     error
	gateway   Execution
	victim    Execution
	replay    Execution
	replayErr error
	// trail is an evidence.jsonl the fake writes itself when the replay runs,
	// as nothing in a working run does; empty writes none.
	trail    string
	journals map[string]string
	// split answers the verifier's runs; nil answers every one as not run.
	split func(service, entrypoint string, args []string) (Split, error)

	broughtUp []string
	wentDown  bool
	ran       [][]string
	env       map[string]string
	// undated names the docker calls that arrived with no deadline, and
	// downErr is what the teardown's own context said when it was called.
	undated []string
	// calls is every docker call in the order it was made.
	calls   []string
	downErr error
	// collector is what the fake collector writes when the replay runs.
	collector string
	// agentTrace is what the fake agent writes when it is asked for a trace.
	agentTrace string
	// exec answers a command run inside a service; execs records each one.
	exec    func(service string, args []string) Split
	execs   [][]string
	stopped []string
	// image answers which image a service's container runs.
	image func(service string) (string, error)
}

// called records whether a docker call carried a deadline, and refuses the call
// when the context is already done, the way exec.CommandContext does. AGENTS.md:
// anything crossing I/O takes a context and carries one. The runner is the
// process that has to survive to write the report.
func (f *fakeCompose) called(ctx context.Context, call string) error {
	if _, ok := ctx.Deadline(); !ok {
		f.undated = append(f.undated, call)
	}
	f.calls = append(f.calls, call)
	return ctx.Err()
}

func (f *fakeCompose) WithEnv(env map[string]string) Compose {
	f.env = env
	return f
}

func (f *fakeCompose) Services(ctx context.Context, _ []string) ([]string, error) {
	if err := f.called(ctx, "services"); err != nil {
		return nil, err
	}
	return f.inProfile, nil
}

func (f *fakeCompose) Build(ctx context.Context, profiles []string) error {
	if err := f.called(ctx, "build"); err != nil {
		return err
	}
	f.built = profiles
	return f.buildErr
}

func (f *fakeCompose) Up(ctx context.Context, _, services []string) error {
	if err := f.called(ctx, "up"); err != nil {
		return err
	}
	f.broughtUp = services
	f.calls[len(f.calls)-1] = "up " + strings.Join(services, " ")
	return f.upErr
}

func (f *fakeCompose) Status(ctx context.Context, _, _ []string) ([]assertion.Service, error) {
	if err := f.called(ctx, "status"); err != nil {
		return nil, err
	}
	return f.status, nil
}

func (f *fakeCompose) Down(ctx context.Context, _ []string) error {
	f.downErr = f.called(ctx, "down")
	f.wentDown = true
	return f.downErr
}

func (f *fakeCompose) RunOnce(ctx context.Context, _ []string, _ string, args []string) (Execution, error) {
	if err := f.called(ctx, "run "+strings.Join(args, " ")); err != nil {
		return Execution{}, err
	}
	f.ran = append(f.ran, args)
	if len(args) == 2 && args[0] == "-probe" {
		if args[1] == "enforcer:8080" {
			return f.gateway, nil
		}
		return f.victim, nil
	}
	f.writeRecords(args)
	return f.replay, f.replayErr
}

// writeRecords plays the part of the collector, the agent and the victims,
// which write into the run directory the runner made for them.
func (f *fakeCompose) writeRecords(args []string) {
	runID := ""
	for i, arg := range args {
		if arg == "-run-id" && i+1 < len(args) {
			runID = args[i+1]
		}
	}
	if runID == "" {
		return
	}
	directory := filepath.Join(f.reports, runID)
	stamp := func(body string) string { return strings.ReplaceAll(body, "${RUN_ID}", runID) }
	if f.trail != "" {
		writeFile(filepath.Join(directory, "evidence.jsonl"), stamp(f.trail))
	}
	if f.collector != "" {
		writeFile(filepath.Join(directory, "collector", "otlp-logs.json"), f.collector)
	}
	if f.agentTrace != "" && slices.Contains(args, "-trace") {
		writeFile(filepath.Join(directory, "agent", "trace.jsonl"), f.agentTrace)
	}
	for victim, body := range f.journals {
		writeFile(filepath.Join(directory, "journals", victim+".jsonl"), stamp(body))
	}
}

func writeFile(path, body string) {
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	_ = os.WriteFile(path, []byte(body), 0o600)
}

// countingSuffix stands in for the random one, so a test can find the run
// directory and two runs in one test do not collide.
func countingSuffix() func() string {
	run := 0
	return func() string {
		run++
		return fmt.Sprintf("run%d", run)
	}
}

// workingCompose answers every docker call the way a healthy run the enforcer
// decides does, down to the trail its collector exports.
func workingCompose(t *testing.T) *fakeCompose {
	t.Helper()
	return &fakeCompose{
		inProfile: []string{"scripted-agent", "enforcer", "collector", "victim-fs"},
		status: []assertion.Service{
			{Name: "enforcer", Running: true, Detail: "running"},
			{Name: "victim-fs", Running: true, Detail: "running"},
		},
		gateway:   Execution{Output: "probe reached enforcer:8080\n"},
		victim:    Execution{ExitCode: 1, Output: "probe unreachable victim-fs:8080: lookup victim-fs: no such host\n"},
		collector: otlpOf(t, evidenceFile),
		journals:  map[string]string{"victim-fs": journalFile},
		exec: func(_ string, args []string) Split {
			if strings.HasSuffix(args[len(args)-1], "/brand") {
				return Split{Stdout: `status 200` + "\n" + `{"version":"` + testPin + `"}`}
			}
			return Split{Stdout: "status 200\n" + drainedHealth}
		},
		image: func(string) (string, error) { return "sha256:aa", nil },
	}
}

// labUnderTest is a lab over a root holding the scenario, its trajectory and
// every file the runner prepares the enforcer from.
func labUnderTest(t *testing.T, compose Compose) (lab, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(filepath.Join(root, "scenarios/flow/flow-01.yaml"), scenarioFile)
	writeFile(filepath.Join(root, "trajectories/flow-01.yaml"), trajectoryFile)
	writeFile(filepath.Join(root, "config/gateway/scenarios/flow-01.yaml"),
		"mode: ENFORCE\nproject_id: project-1\ntenant_id: tenant-1\nlistener:\n  principal:\n    id: user_123\n  agent:\n    id: support-agent\n")
	writeFile(filepath.Join(root, "config/policies/flow-01.json"),
		`{"apiVersion":"agent-policy/v1alpha1","bundle":{"id":"lab-flow-01","version":"1","serial":1}}`)
	writeFile(filepath.Join(root, classification), "- { upstream: victim-fs, tool: fs.read, effect: READ, resource_type: file, resource_from: /path }\n")
	writeFile(filepath.Join(root, fingerprints), "- { upstream: victim-fs, tool: fs.read, fingerprint: \"sha256:0d\" }\n")
	writeFile(filepath.Join(root, versionFile), "ENFORCER_TREE="+testTree+"\n")
	keysDir := t.TempDir()
	writeFile(filepath.Join(keysDir, "public.txt"), "key_id: ed25519-0011223344556677\npublic_key: cHVibGljLWtleQ==\n")
	return lab{
		root:      root,
		workspace: workspace{dir: resolved(root)},
		reports:   filepath.Join(root, "reports"),
		compose:   compose,
		timeout:   defaultScenarioTimeout,
		clock:     func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) },
		suffix:    countingSuffix(),
		log:       io.Discard,
		namespace: testNamespace,
		pin:       testPin,
		keysDir:   keysDir,
		sign: func(_ context.Context, signedWith, policy, outDir string) error {
			if signedWith != keysDir || filepath.Base(policy) != "flow-01.json" {
				t.Errorf("signed %s with %s", policy, signedWith)
			}
			return os.WriteFile(filepath.Join(outDir, "policy.bundle"), []byte("signed"), 0o600)
		},
		drainBound:    2 * time.Second,
		enforcerImage: "lab-enforcer:" + testPin,
		inspect:       inspectEnforcer(testTree),
	}, filepath.Join(root, "scenarios/flow/flow-01.yaml")
}

func TestRunGradesAWholeRunGreenOnlyOnWhatItRead(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)

	report, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if report.Outcome() != assertion.Pass {
		for _, result := range report.Results {
			t.Logf("%s: %s want=%q got=%q %s", result.Check, result.Outcome, result.Want, result.Got, result.Detail)
		}
		t.Fatalf("a whole run reported %s", report.Outcome())
	}
	if slices.Contains(compose.broughtUp, "scripted-agent") {
		t.Error("the one-shot agent was brought up with the long running services")
	}
	if !compose.wentDown {
		t.Error("the profile was left up")
	}
	for _, file := range []string{"junit.xml", "report.md", "boot.json", "probes.log", "replay.log"} {
		if _, err := os.Stat(filepath.Join(subject.reports, report.RunID, file)); err != nil {
			t.Errorf("%s was not written: %v", file, err)
		}
	}
}

func TestRunIsNotGreenWhenSomethingItNeededDidNotHappen(t *testing.T) {
	tests := []struct {
		name  string
		spoil func(*fakeCompose)
		says  string
	}{
		{
			name:  "a service did not start",
			spoil: func(f *fakeCompose) { f.status[1] = assertion.Service{Name: "victim-fs", Detail: "exited (1)"} },
			says:  "boot/victim-fs",
		},
		{
			name:  "the collector exported no trail",
			spoil: func(f *fakeCompose) { f.collector = "" },
			says:  "decisions/step-1",
		},
		{
			name:  "the victim wrote no journal",
			spoil: func(f *fakeCompose) { f.journals = nil },
			says:  "effects/victim-fs",
		},
		{
			name:  "the agent could not be run",
			spoil: func(f *fakeCompose) { f.replayErr = os.ErrNotExist },
			says:  "trajectory/replayed",
		},
		{
			name:  "the probe never ran, so the topology is unproved",
			spoil: func(f *fakeCompose) { f.victim = Execution{ExitCode: 125, Output: "no such image\n"} },
			says:  "network-isolation/victim-unreachable",
		},
		{
			name:  "the agent reached the victim directly",
			spoil: func(f *fakeCompose) { f.victim = Execution{Output: "probe reached victim-fs:8080\n"} },
			says:  "network-isolation/victim-unreachable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, compose, scenario := enforcerLab(t)
			test.spoil(compose)

			report, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if report.Outcome() == assertion.Pass {
				t.Fatalf("a run that %s reported pass", test.name)
			}
			found := false
			for _, result := range report.Results {
				if result.Check == test.says && result.Outcome != assertion.Pass {
					found = true
				}
			}
			if !found {
				for _, result := range report.Results {
					t.Logf("%s: %s %s", result.Check, result.Outcome, result.Detail)
				}
				t.Errorf("nothing named %s went red", test.says)
			}
		})
	}
}

func TestRunRefusesAPairThatDoesNotValidateBeforeBringingAnythingUp(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	// The trajectory gains a step the scenario grades nothing about.
	writeFile(filepath.Join(subject.root, "trajectories/flow-01.yaml"), trajectoryFile+
		"  - call: { server: victim-fs, tool: fs.list, args: { path: \"/data\" } }\n")

	report, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if report.Outcome() != assertion.Fail {
		t.Errorf("a pair that does not validate reported %s", report.Outcome())
	}
	if compose.broughtUp != nil {
		t.Errorf("compose was asked to bring up %v", compose.broughtUp)
	}
	if _, err := os.Stat(filepath.Join(subject.reports, report.RunID, "junit.xml")); err != nil {
		t.Errorf("no report was written for a scenario that could not run: %v", err)
	}
}

func TestRunLeavesTheProfileUpWhenAskedTo(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	subject.keep = true

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if compose.wentDown {
		t.Error("-keep took the profile down")
	}
}

func TestRunProbesFromInsideTheAgentNetwork(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := []string{"enforcer:8080", "victim-fs:8080", "collector:4318", "enforcer:8081"}
	if len(compose.ran) != len(want)+1 {
		t.Fatalf("the agent image was run %d times, want %d probes and one replay", len(compose.ran), len(want))
	}
	for i, target := range want {
		if compose.ran[i][0] != "-probe" || compose.ran[i][1] != target {
			t.Errorf("run %d is %v, want the probe of %s", i+1, compose.ran[i], target)
		}
	}
	if !slices.Contains(compose.ran[len(want)], "-trajectory") {
		t.Errorf("the last run is %v, want the replay", compose.ran[len(want)])
	}
}

// A victim that never started and a victim that served nothing are the two
// answers a denial scenario has to tell apart, and only the journal file tells
// them apart. Read a missing file as an empty one and every `calls_served: {}`
// in the catalogue passes without a victim having run.
func TestRunDoesNotReadAMissingJournalAsAVictimThatServedNothing(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.journals = nil
	writeFile(scenario, strings.Replace(scenarioFile,
		"victim-fs: { calls_served: { fs.read: 1 } }",
		"victim-fs: { calls_served: {} }", 1))

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, result := range graded.Results {
		if result.Check == "effects/victim-fs" && result.Outcome == assertion.Pass {
			t.Errorf("a victim with no journal was read as one that served nothing: %+v", result)
		}
	}
	if graded.Outcome() == assertion.Pass {
		t.Error("a run in which a victim wrote no journal reported pass")
	}
}

// Two runs of one scenario are two runs. Writing into a directory that is
// already there would grade the second one on what the first one left.
func TestRunRefusesARunDirectoryItDidNotJustMake(t *testing.T) {
	subject, _, scenario := enforcerLab(t)
	subject.suffix = func() string { return "same" }

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("the first run: %v", err)
	}
	if _, err := subject.execute(context.Background(), scenario); err == nil {
		t.Error("a second run wrote into the first run's directory")
	}
}

// The enforcer names no run, so an event naming one came from something else;
// a journal line naming another run is that run's. Neither is graded as this
// run's, however complete it looks.
func TestRunIsNotGreenOnRecordsAnotherRunWrote(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.collector = otlpOf(t, strings.ReplaceAll(evidenceFile,
		`"requestId":"r1","projectId"`, `"requestId":"r1","runId":"flow-01-yesterday-0badc0de","projectId"`))
	compose.journals = map[string]string{
		"victim-fs": strings.ReplaceAll(journalFile, "${RUN_ID}", "flow-01-yesterday-0badc0de"),
	}

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() == assertion.Pass {
		t.Error("a run graded on another run's records reported pass")
	}
	named := map[string]bool{"evidence/run-id": false, "decisions/step-1": false, "effects/victim-fs": false}
	for _, result := range graded.Results {
		if _, watched := named[result.Check]; watched && result.Outcome != assertion.Pass {
			named[result.Check] = true
		}
		if result.Check == "evidence/run-id" && !strings.Contains(result.Detail, "flow-01-yesterday-0badc0de") {
			t.Errorf("the result does not name the run the trail belongs to: %+v", result)
		}
	}
	for check, red := range named {
		if !red {
			t.Errorf("%s did not go red on another run's records", check)
		}
	}
}

// A trail that could not be read and a trail with nothing in it are two facts.
// Reported as one, they send the person reading a red run to a file that turns
// out to be full, and the run directory keeps no trace of the real cause.
func TestRunSaysWhenTheTrailCouldNotBeRead(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	compose.collector = ""
	compose.trail = `{"eventId":"e1","kind":"EVENT_KIND_ACTION_PROPOSED"` + "\n"

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() == assertion.Pass {
		t.Fatal("a run whose trail could not be read reported pass")
	}
	trail := assertion.Result{}
	for _, result := range graded.Results {
		if result.Check == "evidence/trail" {
			trail = result
		}
	}
	if trail.Check == "" {
		t.Fatal("nothing reported on the trail the runner could not read")
	}
	if trail.Outcome != assertion.Indeterminate {
		t.Errorf("a trail that could not be read is %s, want indeterminate (%+v)", trail.Outcome, trail)
	}
	if trail.Detail == "" || strings.Contains(trail.Got, "no events") {
		t.Errorf("the read failure is reported as an empty trail: %+v", trail)
	}
}

// The runner is the process that has to survive to write the report. A compose
// up against a healthcheck that never passes, or a run against a container that
// hangs, would otherwise wedge one scenario with no junit.xml and no report.md
// — and under -all, every scenario after it.
func TestRunGivesEveryDockerCallADeadline(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(compose.undated) > 0 {
		t.Errorf("docker was called with no deadline: %s", strings.Join(compose.undated, ", "))
	}
}

// The scenario whose deadline just fired is exactly the one whose containers
// have to come down, so the teardown does not inherit the deadline that fired.
func TestRunTakesTheProfileDownAfterItsDeadlineHasPassed(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	subject.timeout = time.Nanosecond

	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() == assertion.Pass {
		t.Error("a run whose deadline passed reported pass")
	}
	if !compose.wentDown {
		t.Fatal("the profile was left up after the run's deadline passed")
	}
	if compose.downErr != nil {
		t.Errorf("the teardown inherited the deadline that fired: %v", compose.downErr)
	}
}
