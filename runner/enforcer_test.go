package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
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

// enforcerLab is a working lab and the fake docker it runs on.
func enforcerLab(t *testing.T) (lab, *fakeCompose, string) {
	t.Helper()
	compose := workingCompose(t)
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
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
	for _, name := range []string{
		"plane/version", "plane/drained", "evidence/run-id", "decisions/step-1",
		"evidence/enforcement-mode", "evidence/executed-digest",
	} {
		if found[name].Outcome != assertion.Pass {
			t.Errorf("%s is %s", name, found[name].Outcome)
		}
	}
	if !strings.Contains(found["evidence/run-id"].Got, "01PLANERUN") {
		t.Errorf("the trail's run was not the one it names: %q", found["evidence/run-id"].Got)
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
	for _, made := range []string{"pki", "collector", collectorTLSDir, exportCADir} {
		if info, err := os.Stat(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], made)); err != nil || !info.IsDir() {
			t.Errorf("the run directory holds no %s/ for the services to write into: %v", made, err)
		}
	}
	tls := filepath.Join(compose.env["LAB_RUN_HOST_DIR"], collectorTLSDir)
	if _, err := os.Stat(filepath.Join(tls, "key.pem")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the collector's key outlived the run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tls, "cert.pem")); err != nil {
		t.Errorf("the collector's certificate is gone with its key: %v", err)
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
