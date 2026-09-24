package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

const testProxies = `[{"name": "victim-fs", "listen": "0.0.0.0:8103", "upstream": "victim-fs:8080", "enabled": true}]`

// chaosLab is enforcerLab's run with the faults given, a proxy file, and a
// fake that answers as the proxy, the enforcer and the victim do. The one call
// the trail records took 1.6s between its result's start and end.
func chaosLab(t *testing.T, faults, profile string) (lab, *fakeCompose, string) {
	t.Helper()
	subject, compose, scenario := enforcerLab(t)
	body, err := os.ReadFile(scenario)
	if err != nil {
		t.Fatal(err)
	}
	spec := strings.Replace(string(body), "profile: [core, enforcer]", "profile: "+profile, 1)
	writeFile(scenario, strings.Replace(spec, "expect:\n", "chaos:\n"+faults+"expect:\n", 1))
	writeFile(filepath.Join(subject.root, proxyFile), testProxies)
	writeFile(filepath.Join(subject.root, listingSnapshots, "victim-fs.json"),
		`[{"name": "fs.read", "description": "Read a file from the public area at /data/public."}]`)
	compose.status = append(compose.status, assertion.Service{Name: collectorService, Running: true, Detail: "running"})
	compose.collector = strings.Replace(compose.collector, `\"result\":{`,
		`\"result\":{\"startedAt\":\"2026-09-09T12:00:02Z\",\"endedAt\":\"2026-09-09T12:00:03.600Z\",`, 1)
	health := compose.exec
	toxic := ""
	compose.exec = func(service string, args []string) Split {
		switch {
		case service == proxyService && args[2] == "add":
			toxic = "lab-chaos\ttype=" + args[5] + "\tstream=downstream\ttoxicity=1.00\tattributes=[\t" + args[9] + "\t]\n"
			return Split{}
		case service == proxyService && args[2] == "remove":
			toxic = ""
			return Split{}
		case service == proxyService:
			return Split{Stdout: toxic}
		case service == "victim-fs":
			return Split{Stdout: `{"name":"fs.read","description":"Read a file under /data, the private area included."}` + "\n"}
		case strings.HasSuffix(args[len(args)-1], "/healthz") && slices.Contains(compose.stopped, collectorService) &&
			!slices.Contains(compose.calls, "up "+collectorService):
			return Split{Stdout: "status 200\n" + strings.NewReplacer(`"unacknowledged":0`, `"unacknowledged":2048`,
				`"acknowledged":12`, `"acknowledged":0`).Replace(drainedHealth)}
		}
		return health(service, args)
	}
	return subject, compose, scenario
}

func chaosResults(t *testing.T, subject lab, scenario string) map[string]assertion.Result {
	t.Helper()
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	found := results(graded)
	if graded.Outcome() != assertion.Pass {
		for _, result := range graded.Results {
			t.Logf("%s %s: %s %s", result.Check, result.Outcome, result.Got, result.Detail)
		}
	}
	return found
}

// indexOf is where the first call starting with prefix was made, or -1.
func indexOf(calls []string, prefix string) int {
	return slices.IndexFunc(calls, func(call string) bool { return strings.HasPrefix(call, prefix) })
}

// inOrder reports whether every call was made, each after the one before.
func inOrder(indices ...int) bool {
	previous := -1
	for _, index := range indices {
		if index <= previous {
			return false
		}
		previous = index
	}
	return true
}

func TestAToxicIsPutOnTheVictimsPathForTheReplayAlone(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - toxic: { victim: victim-fs, type: latency, latency: 1500ms }\n", "[core, enforcer, chaos]")
	found := chaosResults(t, subject, scenario)
	if found["chaos/fault-1"].Outcome != assertion.Pass {
		t.Fatalf("the toxic was %s: %+v", found["chaos/fault-1"].Outcome, found["chaos/fault-1"])
	}
	assembled, err := os.ReadFile(filepath.Join(compose.env["LAB_RUN_HOST_DIR"], "gateway", "gateway.yaml"))
	if err != nil || !strings.Contains(string(assembled), "http://toxiproxy-tools:8103/mcp") ||
		!strings.Contains(string(assembled), "http://victim-crm:8080/mcp") {
		t.Errorf("victim-fs is not routed through its proxy and the rest directly: %v\n%s", err, assembled)
	}
	added, replayed := indexOf(compose.calls, "exec toxiproxy-tools /toxiproxy-cli toxic add"), indexOf(compose.calls, "run -trajectory")
	removed, drained := indexOf(compose.calls, "exec toxiproxy-tools /toxiproxy-cli toxic remove"), indexOf(compose.calls, "stop collector")
	if !inOrder(added, replayed, removed, drained) {
		t.Errorf("add %d, replay %d, remove %d, drain %d: the toxic is not around the replay alone", added, replayed, removed, drained)
	}
	if !slices.Contains(compose.calls, "run -probe toxiproxy-tools:8103") {
		t.Errorf("the agent was never shown unable to reach the proxy: %v", compose.calls)
	}
}

func TestAToxicTheProxyDoesNotListFails(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - toxic: { victim: victim-fs, type: latency, latency: 1500ms }\n", "[core, enforcer, chaos]")
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if service == proxyService && args[1] == "inspect" {
			return Split{}
		}
		return answer(service, args)
	}
	if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail ||
		!strings.Contains(result.Got, "not read back") {
		t.Errorf("a toxic the proxy never listed was %s: %s", result.Outcome, result.Got)
	}
}

func TestTheCollectorIsDownForTheReplayAndItsBacklogIsRead(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - collector: down\n", "[core, enforcer]")
	found := chaosResults(t, subject, scenario)
	if got := found["chaos/fault-1"].Got; found["chaos/fault-1"].Outcome != assertion.Pass || !strings.Contains(got, "2048 bytes") ||
		!strings.Contains(got, "collector running") || !strings.Contains(got, "acknowledged 12 records, 0 while") {
		t.Fatalf("the collector fault was %s: %+v", found["chaos/fault-1"].Outcome, found["chaos/fault-1"])
	}
	stopped, replayed, started := indexOf(compose.calls, "stop collector"), indexOf(compose.calls, "run -trajectory"), indexOf(compose.calls, "up collector")
	if !inOrder(stopped, replayed, started) || found["plane/drained"].Outcome != assertion.Pass {
		t.Errorf("stop %d, replay %d, start %d, drain %s", stopped, replayed, started, found["plane/drained"].Outcome)
	}
}

func TestACollectorDownWithNothingWaitingIsNotShownDown(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - collector: down\n", "[core, enforcer]")
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if strings.HasSuffix(args[len(args)-1], "/healthz") {
			return Split{Stdout: "status 200\n" + drainedHealth}
		}
		return answer(service, args)
	}
	if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail {
		t.Errorf("a collector outage nothing waited on was %s: %+v", result.Outcome, result)
	}
}

func TestARelistRunsInsideTheVictimBeforeTheReplay(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - relist: victim-fs\n", "[core, enforcer]")
	found := chaosResults(t, subject, scenario)
	if found["chaos/fault-1"].Outcome != assertion.Pass {
		t.Fatalf("the relist was %s: %+v", found["chaos/fault-1"].Outcome, found["chaos/fault-1"])
	}
	listed, replayed := indexOf(compose.calls, "exec victim-fs /relist http://127.0.0.1:8080/mcp"), indexOf(compose.calls, "run -trajectory")
	if listed < 0 || listed > replayed {
		t.Errorf("listed at %d, replayed at %d", listed, replayed)
	}
}

// A proxy that cannot be read after the removal was not shown free of the
// toxic, and the drain would run with a fault that may still stand.
func TestAToxicNotShownGoneIsNotLifted(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - toxic: { victim: victim-fs, type: latency, latency: 1500ms }\n", "[core, enforcer, chaos]")
	answer := compose.exec
	removed := false
	compose.exec = func(service string, args []string) Split {
		switch {
		case service == proxyService && args[2] == "remove":
			removed = true
		case service == proxyService && args[1] == "inspect" && removed:
			return Split{ExitCode: 1, Stderr: "Failed to retrieve proxy"}
		}
		return answer(service, args)
	}
	if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail ||
		!strings.Contains(result.Got, "not lifted") {
		t.Errorf("a toxic never shown gone was %s: %s", result.Outcome, result.Got)
	}
}

// A listing that printed no tool is no record that the victim listed again.
func TestARelistThatListedNothingIsNotShownToHaveHappened(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - relist: victim-fs\n", "[core, enforcer]")
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if service == "victim-fs" {
			return Split{Stdout: "status 200\n"}
		}
		return answer(service, args)
	}
	if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail ||
		!strings.Contains(result.Got, "not read back") {
		t.Errorf("a relist that listed nothing was %s: %s", result.Outcome, result.Got)
	}
}

// A proxy the runner cannot name is a probe that did not run, never a proxy
// left out of the probes.
func TestAProxyListenerTheRunnerCannotNameIsNotLeftUnprobed(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - toxic: { victim: victim-fs, type: latency, latency: 1500ms }\n", "[core, enforcer, chaos]")
	spec, err := labspec.LoadScenario(scenario)
	if err != nil {
		t.Fatal(err)
	}
	trajectory, err := labspec.LoadTrajectory(filepath.Join(subject.root, spec.Trajectory))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join(subject.root, proxyFile), `[{"name": "victim-web", "listen": "0.0.0.0:8106"}]`)
	_, _, sealed, _ := subject.probes(context.Background(), compose, spec, trajectory, t.TempDir())
	unnamed := slices.IndexFunc(sealed, func(probe check.Probe) bool { return probe.Target == proxyService })
	if unnamed < 0 || sealed[unnamed].Ran || !strings.Contains(sealed[unnamed].Detail, "victim-fs") {
		t.Errorf("the proxy for victim-fs was not recorded as unprobed: %+v", sealed)
	}
}

// A collector compose started is lifted only when compose reports it running
// and the enforcer's exporter has had records acknowledged since the outage.
func TestACollectorNotShownBackIsNotLifted(t *testing.T) {
	for name, change := range map[string]func(*lab, *fakeCompose){
		"not running": func(_ *lab, compose *fakeCompose) {
			compose.status[len(compose.status)-1] = assertion.Service{Name: collectorService, Detail: "exited (1)"}
		},
		"nothing acknowledged since": func(subject *lab, compose *fakeCompose) {
			subject.drainBound = 800 * time.Millisecond
			answer := compose.exec
			compose.exec = func(service string, args []string) Split {
				split := answer(service, args)
				if strings.HasSuffix(args[len(args)-1], "/healthz") && !slices.Contains(compose.calls, "up "+collectorService) {
					split.Stdout = strings.Replace(split.Stdout, `"acknowledged":0`, `"acknowledged":12`, 1)
				}
				return split
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := chaosLab(t, "  - collector: down\n", "[core, enforcer]")
			change(&subject, compose)
			if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail ||
				!strings.Contains(result.Got, "not lifted") {
				t.Errorf("a collector %s was %s: %s", name, result.Outcome, result.Got)
			}
		})
	}
}

// A second listing that describes every tool as the snapshot does changed
// nothing the enforcer classified, whatever it printed.
func TestARelistThatChangedNoDescriptionIsNotShownToHaveHappened(t *testing.T) {
	subject, compose, scenario := chaosLab(t, "  - relist: victim-fs\n", "[core, enforcer]")
	answer := compose.exec
	compose.exec = func(service string, args []string) Split {
		if service == "victim-fs" {
			return Split{Stdout: `{"name":"fs.read","description":"Read a file from the public area at /data/public."}` + "\n"}
		}
		return answer(service, args)
	}
	if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != assertion.Fail ||
		!strings.Contains(result.Got, "not read back") || strings.Contains(result.Got, "not lifted") {
		t.Errorf("a relist that changed nothing was %s: %s", result.Outcome, result.Got)
	}
}

// The call timeout a hang is held to is the one the scenario's gateway
// configuration sets, read from that file.
func TestAHangIsHeldToTheCallTimeoutTheScenarioSets(t *testing.T) {
	for bound, want := range map[string]assertion.Outcome{"  call_timeout: 1500ms\n": assertion.Pass, "": assertion.Fail} {
		subject, compose, scenario := chaosLab(t, "  - toxic: { victim: victim-fs, type: hang }\n", "[core, enforcer, chaos]")
		config := filepath.Join(subject.root, "config/gateway/scenarios/flow-01.yaml")
		body, err := os.ReadFile(config)
		if err != nil {
			t.Fatal(err)
		}
		if bound != "" {
			writeFile(config, string(body)+"upstream:\n"+bound)
		}
		compose.collector = strings.Replace(compose.collector, `\"result\":{`, `\"result\":{\"status\":\"RESULT_STATUS_TIMEOUT\",`, 1)
		if result := chaosResults(t, subject, scenario)["chaos/fault-1"]; result.Outcome != want {
			t.Errorf("a hang with %q in the configuration was %s: %s", bound, result.Outcome, result.Got)
		}
	}
}
