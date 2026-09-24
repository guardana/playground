package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
)

const (
	testPin       = "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"
	testNamespace = "guardana.control"
)

// otlpOf wraps trail lines the way the collector's file exporter writes them:
// one export request per line, each record's body one event, its attributes
// repeating the identifiers under the namespace.
func otlpOf(t *testing.T, lines string) string {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(lines), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		var attributes []map[string]any
		for key, field := range map[string]string{
			"event_id": "eventId", "request_id": "requestId", "run_id": "runId", "project_id": "projectId",
			"tenant_id": "tenantId", "kind": "kind", "enforcement_mode": "enforcementMode",
		} {
			value, _ := event[field].(string)
			if value == "" && field == "enforcementMode" {
				value = "ENFORCEMENT_MODE_UNSPECIFIED"
			}
			if value != "" {
				attributes = append(attributes, map[string]any{
					"key": testNamespace + "." + key, "value": map[string]any{"stringValue": value}})
			}
		}
		records = append(records, map[string]any{"body": map[string]any{"stringValue": line}, "attributes": attributes})
	}
	request := map[string]any{"resourceLogs": []any{map[string]any{"scopeLogs": []any{map[string]any{"logRecords": records}}}}}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded) + "\n"
}

// enforcerLab is a lab whose scenario the enforcer decides, with every file the
// runner prepares from written into the test's own root.
func enforcerLab(t *testing.T) (lab, *fakeCompose, string) {
	t.Helper()
	compose := workingCompose("")
	compose.trail = ""
	unstamped := strings.NewReplacer(`"runId":"${RUN_ID}",`, "", `"runId":"${RUN_ID}"`, "").Replace(evidenceFile)
	compose.collector = otlpOf(t, unstamped)
	compose.exec = func(_ string, args []string) Split {
		switch {
		case strings.HasSuffix(args[len(args)-1], "/brand"):
			return Split{Stdout: `status 200` + "\n" + `{"version":"` + testPin + `"}`}
		default:
			return Split{Stdout: "status 200\n" + drainedHealth}
		}
	}
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	root := subject.root
	body := strings.Replace(scenarioFile, "stub: { verdicts: config/scenarios/flow-01.yaml }\n",
		"gateway: { config: config/gateway/scenarios/flow-01.yaml, policy: config/policies/flow-01.json }\n", 1)
	writeFile(scenario, strings.Replace(body, "profile: [core, stub]", "profile: [core, enforcer]", 1))
	writeFile(filepath.Join(root, "config/gateway/scenarios/flow-01.yaml"),
		"mode: ENFORCE\nproject_id: project-1\ntenant_id: tenant-1\nlistener:\n  principal:\n    id: user_123\n  agent:\n    id: support-agent\n")
	writeFile(filepath.Join(root, "config/policies/flow-01.json"),
		`{"apiVersion":"agent-policy/v1alpha1","bundle":{"id":"lab-flow-01","version":"1","serial":1}}`)
	writeFile(filepath.Join(root, classification), "- { upstream: victim-fs, tool: fs.read, effect: READ, resource_type: file, resource_from: /path }\n")
	writeFile(filepath.Join(root, fingerprints), "- { upstream: victim-fs, tool: fs.read, fingerprint: \"sha256:0d\" }\n")
	subject.keysDir = t.TempDir()
	writeFile(filepath.Join(subject.keysDir, "public.txt"), "key_id: ed25519-0011223344556677\npublic_key: cHVibGljLWtleQ==\n")
	subject.sign = func(_ context.Context, keysDir, policy, outDir string) error {
		if keysDir != subject.keysDir || filepath.Base(policy) != "flow-01.json" {
			t.Errorf("signed %s with %s", policy, keysDir)
		}
		return os.WriteFile(filepath.Join(outDir, "policy.bundle"), []byte("signed"), 0o600)
	}
	subject.namespace, subject.pin = testNamespace, testPin
	subject.drainBound = 2 * time.Second
	compose.image = func(string) (string, error) { return "sha256:aa", nil }
	subject.enforcerImage = "lab-enforcer:" + testPin
	subject.inspect = func(context.Context, string, ...string) (string, error) {
		return "sha256:aa " + testPin, nil
	}
	return subject, compose, scenario
}

func results(graded assertion.Report) map[string]assertion.Result {
	found := map[string]assertion.Result{}
	for _, result := range graded.Results {
		found[result.Check] = result
	}
	return found
}

func TestARunTheEnforcerDecidesIsGradedFromTheCollectorsTrail(t *testing.T) {
	subject, _, scenario := enforcerLab(t)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if graded.Outcome() != assertion.Pass {
		for _, result := range graded.Results {
			t.Logf("%s %s: %s %s", result.Check, result.Outcome, result.Got, result.Detail)
		}
		t.Fatalf("the run is %s", graded.Outcome())
	}
	found := results(graded)
	for _, name := range []string{"plane/version", "plane/drained", "evidence/run-id", "decisions/step-1"} {
		if found[name].Outcome != assertion.Pass {
			t.Errorf("%s is %s", name, found[name].Outcome)
		}
	}
	if !strings.Contains(found["evidence/run-id"].Want, "no event names a run") {
		t.Errorf("the trail was graded as a stamped one: %q", found["evidence/run-id"].Want)
	}
}

func TestAnEnforcerRunIsPreparedAndDrainedThroughTheCollector(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	if _, err := subject.execute(context.Background(), scenario); err != nil {
		t.Fatalf("execute: %v", err)
	}
	directory := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "gateway")
	assembled, err := os.ReadFile(filepath.Join(directory, "gateway.yaml"))
	if err != nil || !strings.Contains(string(assembled), `bundle_id: "lab-flow-01"`) {
		t.Errorf("no assembled configuration under %q: %v\n%s", directory, err, assembled)
	}
	if replay := strings.Join(compose.ran[len(compose.ran)-1], " "); !strings.Contains(replay, "-gateway http://enforcer:8080/mcp") {
		t.Errorf("the agent was pointed elsewhere: %s", replay)
	}
	if len(compose.stopped) != 1 || compose.stopped[0] != "collector" {
		t.Errorf("the collector was not stopped before its file was read: %v", compose.stopped)
	}
	for _, made := range []string{"pki", "collector"} {
		if info, err := os.Stat(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], made)); err != nil || !info.IsDir() {
			t.Errorf("the run directory holds no %s/ for the services to write into: %v", made, err)
		}
	}
}

func TestAStaleEnforcerImageIsNotThePin(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if strings.HasSuffix(args[len(args)-1], "/brand") {
			return Split{Stdout: "status 200\n" + `{"version":"dev"}`}
		}
		return answer(service, args)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/version"]; result.Outcome != assertion.Fail || result.Got != "dev" {
		t.Errorf("an enforcer reporting dev was %s: %+v", result.Outcome, result)
	}
}

func TestATrailReadBeforeTheSpoolDrainedFails(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if strings.HasSuffix(args[len(args)-1], "/healthz") {
			return Split{Stdout: "status 200\n" + strings.Replace(drainedHealth, `"unacknowledged":0`, `"unacknowledged":4096`, 1)}
		}
		return answer(service, args)
	}
	subject.drainBound = 800 * time.Millisecond
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := results(graded)
	if found["plane/drained"].Outcome != assertion.Fail || !strings.Contains(found["plane/drained"].Got, "4096") {
		t.Errorf("a spool that never drained was %s: %s", found["plane/drained"].Outcome, found["plane/drained"].Got)
	}
	if graded.Outcome() == assertion.Pass {
		t.Error("a run whose trail was never drained passed")
	}
}

func TestARunWithNoLabKeyIsRefusedBeforeAnythingBoots(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	subject.keysDir = filepath.Join(t.TempDir(), "absent")
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/prepared"]; result.Outcome != assertion.Fail || !strings.Contains(result.Detail, "make lab-key") {
		t.Errorf("a run with no lab key was %s: %+v", result.Outcome, result)
	}
	if len(compose.broughtUp) != 0 {
		t.Errorf("a refused run brought up %v", compose.broughtUp)
	}
}
