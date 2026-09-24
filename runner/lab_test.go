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
profile: [core]
enforcement_mode: enforce
trajectory: trajectories/flow-01.yaml
stub: { verdicts: config/scenarios/flow-01.yaml }
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
	// ${RUN_ID} is what the fake stub gateway and the fake victims stamp their
	// records with, the way the real ones stamp LAB_RUN_ID. A record naming
	// another run is a record of another run, and the runner has to say so.
	evidenceFile = `{"eventId":"e1","kind":"EVENT_KIND_ACTION_PROPOSED","requestId":"r1","runId":"${RUN_ID}","occurredAt":"2026-09-09T12:00:00Z","proposed":{"requestId":"r1","context":{"runId":"${RUN_ID}","stepId":"1"}}}
{"eventId":"e2","kind":"EVENT_KIND_POLICY_DECIDED","requestId":"r1","runId":"${RUN_ID}","occurredAt":"2026-09-09T12:00:01Z","prevEventId":"e1","decision":{"requestId":"r1","verdict":"VERDICT_ALLOW","reasonCodes":["RULE_ALLOW"],"policyBundleDigest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"}}
{"eventId":"e3","kind":"EVENT_KIND_ACTION_STARTED","requestId":"r1","runId":"${RUN_ID}","occurredAt":"2026-09-09T12:00:02Z","prevEventId":"e2"}
{"eventId":"e4","kind":"EVENT_KIND_ACTION_COMPLETED","requestId":"r1","runId":"${RUN_ID}","occurredAt":"2026-09-09T12:00:03Z","prevEventId":"e3"}
`
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
	upErr     error
	gateway   Execution
	victim    Execution
	replay    Execution
	replayErr error
	// what the fake writes when the replay runs; nil writes nothing.
	trail    string
	journals map[string]string

	broughtUp []string
	wentDown  bool
	ran       [][]string
	env       map[string]string
	// undated names the docker calls that arrived with no deadline, and
	// downErr is what the teardown's own context said when it was called.
	undated []string
	downErr error
}

// called records whether a docker call carried a deadline, and refuses the call
// when the context is already done, the way exec.CommandContext does. AGENTS.md:
// anything crossing I/O takes a context and carries one. The runner is the
// process that has to survive to write the report.
func (f *fakeCompose) called(ctx context.Context, call string) error {
	if _, ok := ctx.Deadline(); !ok {
		f.undated = append(f.undated, call)
	}
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

func (f *fakeCompose) Up(ctx context.Context, _, services []string) error {
	if err := f.called(ctx, "up"); err != nil {
		return err
	}
	f.broughtUp = services
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
		if strings.HasPrefix(args[1], "stub-gateway") {
			return f.gateway, nil
		}
		return f.victim, nil
	}
	f.writeRecords(args)
	return f.replay, f.replayErr
}

// writeRecords plays the part of the stub gateway and the victims, which write
// into the run directory the runner made for them.
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

func workingCompose(reports string) *fakeCompose {
	return &fakeCompose{
		reports:   reports,
		inProfile: []string{"scripted-agent", "stub-gateway", "victim-fs"},
		status: []assertion.Service{
			{Name: "stub-gateway", Running: true, Detail: "running"},
			{Name: "victim-fs", Running: true, Detail: "running"},
		},
		gateway:  Execution{Output: "probe reached stub-gateway:8080\n"},
		victim:   Execution{ExitCode: 1, Output: "probe unreachable victim-fs:8080: lookup victim-fs: no such host\n"},
		trail:    evidenceFile,
		journals: map[string]string{"victim-fs": journalFile},
	}
}

func labUnderTest(t *testing.T, compose Compose) (lab, string) {
	t.Helper()
	root := t.TempDir()
	writeFile(filepath.Join(root, "scenarios/flow/flow-01.yaml"), scenarioFile)
	writeFile(filepath.Join(root, "trajectories/flow-01.yaml"), trajectoryFile)
	reports := filepath.Join(root, "reports")
	return lab{
		root:    root,
		reports: reports,
		compose: compose,
		timeout: defaultScenarioTimeout,
		clock:   func() time.Time { return time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) },
		suffix:  countingSuffix(),
		log:     io.Discard,
	}, filepath.Join(root, "scenarios/flow/flow-01.yaml")
}

func TestRunGradesAWholeRunGreenOnlyOnWhatItRead(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports

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
			name:  "the trail was never written",
			spoil: func(f *fakeCompose) { f.trail = "" },
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
			root := t.TempDir()
			compose := workingCompose(filepath.Join(root, "reports"))
			subject, scenario := labUnderTest(t, compose)
			compose.reports = subject.reports
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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	subject.keep = true

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if compose.wentDown {
		t.Error("-keep took the profile down")
	}
}

func TestRunProbesFromInsideTheAgentNetwork(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(compose.ran) != 3 {
		t.Fatalf("the agent image was run %d times, want two probes and one replay", len(compose.ran))
	}
	if compose.ran[0][0] != "-probe" || !strings.HasPrefix(compose.ran[0][1], "stub-gateway") {
		t.Errorf("the first run is %v, want the gateway probe", compose.ran[0])
	}
	if compose.ran[1][0] != "-probe" || !strings.HasPrefix(compose.ran[1][1], "victim-fs") {
		t.Errorf("the second run is %v, want a victim probe", compose.ran[1])
	}
	if !slices.Contains(compose.ran[2], "-trajectory") {
		t.Errorf("the third run is %v, want the replay", compose.ran[2])
	}
}

// A victim that never started and a victim that served nothing are the two
// answers a denial scenario has to tell apart, and only the journal file tells
// them apart. Read a missing file as an empty one and every `calls_served: {}`
// in the catalogue passes without a victim having run.
func TestRunDoesNotReadAMissingJournalAsAVictimThatServedNothing(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	subject.suffix = func() string { return "same" }

	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("the first run: %v", err)
	}
	if _, err := subject.execute(context.Background(), scenario); err == nil {
		t.Error("a second run wrote into the first run's directory")
	}
}

// The run directory is fresh per run, and a directory is luck rather than an
// assertion. A complete trail and a full journal left by yesterday's run
// satisfy every other check, so the records have to say which run wrote them.
func TestRunIsNotGreenOnRecordsAnotherRunWrote(t *testing.T) {
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	compose.trail = strings.ReplaceAll(evidenceFile, "${RUN_ID}", "flow-01-yesterday-0badc0de")
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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports

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
	root := t.TempDir()
	compose := workingCompose(filepath.Join(root, "reports"))
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
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
