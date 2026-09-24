package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/gateway"
)

// What the runner and the chaos proxy agree on.
const (
	proxyService = "toxiproxy-tools"
	proxyFile    = "compose/toxiproxy/proxies.json"
	proxyCLI     = "/toxiproxy-cli"
	// toxicName is the one toxic the runner puts on a victim's proxy.
	toxicName = "lab-chaos"
	// listingSnapshots holds each victim's tools as classify-victims listed them.
	listingSnapshots = "config/gateway/tools"
)

// proxyTargets reads which proxy listener stands for each victim, from the
// file the proxy starts with, as host:port on tool-net.
func (l lab) proxyTargets() (map[string]string, error) {
	body, err := os.ReadFile(filepath.Join(l.root, proxyFile)) // #nosec G304 -- the lab's own file.
	if err != nil {
		return nil, err
	}
	var proxies []struct {
		Name   string `json:"name"`
		Listen string `json:"listen"`
	}
	if err := json.Unmarshal(body, &proxies); err != nil {
		return nil, fmt.Errorf("%s: %w", proxyFile, err)
	}
	targets := map[string]string{}
	for _, proxy := range proxies {
		_, port, found := strings.Cut(proxy.Listen, ":")
		if !found || port == "" {
			return nil, fmt.Errorf("%s: proxy %s listens on %q, which names no port", proxyFile, proxy.Name, proxy.Listen)
		}
		targets[proxy.Name] = proxyService + ":" + port
	}
	return targets, nil
}

// routedUpstreams is every victim the enforcer fronts, a proxied one reached
// through its proxy and the rest directly.
func (l lab) routedUpstreams(proxied []string) ([]gateway.Upstream, error) {
	var targets map[string]string
	if len(proxied) > 0 {
		found, err := l.proxyTargets()
		if err != nil {
			return nil, err
		}
		targets = found
	}
	routed := make([]gateway.Upstream, 0, len(victims()))
	for _, name := range victims() {
		routed = append(routed, gateway.Upstream{Name: name, Endpoint: "http://" + name + ":" + servicePort + "/mcp"})
	}
	for _, name := range proxied {
		target, found := targets[name]
		index := victimIndex(name)
		if !found || index < 0 {
			return nil, fmt.Errorf("a toxic on %s, which no proxy in %s fronts", name, proxyFile)
		}
		routed[index].Endpoint = "http://" + target + "/mcp"
	}
	return routed, nil
}

func victimIndex(name string) int {
	for i, victim := range victims() {
		if victim == name {
			return i
		}
	}
	return -1
}

// sealedProxies are the proxy listeners a run routes through, which the agent
// must not reach any more than the victims behind them.
func (l lab) sealedProxies(spec labspec.Scenario) ([]string, error) {
	proxied := spec.Proxied()
	if len(proxied) == 0 {
		return nil, nil
	}
	targets, err := l.proxyTargets()
	if err != nil {
		return nil, err
	}
	sealed := make([]string, 0, len(proxied))
	for _, name := range proxied {
		target, found := targets[name]
		if !found {
			return nil, fmt.Errorf("no proxy in %s fronts %s", proxyFile, name)
		}
		sealed = append(sealed, target)
	}
	return sealed, nil
}

// chaosRun is what applying and lifting a scenario's faults left behind.
type chaosRun struct {
	faults []check.ChaosFault
	log    strings.Builder
}

// applyChaos breaks what the scenario names, in order, after boot and before
// the replay, and records each step in chaos.log.
func (l lab) applyChaos(ctx context.Context, compose Compose, spec labspec.Scenario) *chaosRun {
	run := &chaosRun{}
	for _, fault := range spec.Chaos {
		recorded := check.ChaosFault{Name: faultName(fault)}
		switch {
		case fault.Toxic != nil:
			l.applyToxic(ctx, compose, spec, *fault.Toxic, &recorded, &run.log)
		case fault.Collector != "":
			err := compose.Stop(ctx, spec.Profile, collectorService)
			recorded.Applied = err == nil
			fmt.Fprintf(&run.log, "collector stopped: %v\n", err)
		default:
			recorded.Unliftable = true
			l.relist(ctx, compose, spec.Profile, fault.Relist, &recorded, &run.log)
		}
		run.faults = append(run.faults, recorded)
	}
	return run
}

// applyToxic names what the trail has to show of the toxic, then puts it on
// the victim's proxy. A hang is held to the call timeout the scenario's
// gateway configuration sets.
func (l lab) applyToxic(
	ctx context.Context, compose Compose, spec labspec.Scenario, toxic labspec.Toxic, fault *check.ChaosFault, log *strings.Builder,
) {
	fault.Victim = toxic.Victim
	switch toxic.Type {
	case labspec.ToxicLatency:
		fault.Latency = time.Duration(toxic.Latency)
	case labspec.ToxicHang:
		fault.Hang = true
		partial, err := os.ReadFile(filepath.Join(l.root, spec.Gateway.Config)) // #nosec G304 -- a repository path the scenario names.
		if err == nil {
			fault.CallTimeout, err = gateway.CallTimeout(partial)
		}
		if err != nil {
			noteRead(fault, "upstream.call_timeout unread: %v", err)
		}
	}
	l.addToxic(ctx, compose, spec.Profile, toxic, fault, log)
}

// liftChaos mends every fault after the replay and before the drain: toxics
// removed, the collector's backlog read and the collector started again. A
// second listing has nothing to lift.
func (l lab) liftChaos(ctx context.Context, compose Compose, spec labspec.Scenario, run *chaosRun, runDir string) check.Chaos {
	for i, fault := range spec.Chaos {
		recorded := &run.faults[i]
		switch {
		case fault.Toxic != nil:
			l.removeToxic(ctx, compose, spec.Profile, fault.Toxic.Victim, recorded, &run.log)
		case fault.Collector != "":
			l.restartCollector(ctx, compose, spec.Profile, recorded, &run.log)
		}
	}
	source := filepath.Join(runDir, "chaos.log")
	// #nosec G703 -- the path is inside the run directory the runner made.
	if err := os.WriteFile(source, []byte(run.log.String()), 0o600); err != nil {
		l.note("writing the chaos record: %v", err)
	}
	return check.Chaos{Faults: run.faults, Source: source}
}

// noteRead adds one thing the runner read about a fault to what the report
// says was read.
func noteRead(fault *check.ChaosFault, format string, values ...any) {
	if fault.Detail != "" {
		fault.Detail += "; "
	}
	fault.Detail += fmt.Sprintf(format, values...)
}

func faultName(fault labspec.Fault) string {
	switch {
	case fault.Toxic != nil && fault.Toxic.Type == labspec.ToxicLatency:
		return fmt.Sprintf("latency %s on %s's answers", time.Duration(fault.Toxic.Latency), fault.Toxic.Victim)
	case fault.Toxic != nil:
		return "a hang on " + fault.Toxic.Victim + "'s answers"
	case fault.Collector != "":
		return "the collector down"
	}
	return "a second listing of " + fault.Relist + "'s tools"
}

// toxicArgs is the proxy's spelling of a toxic on a victim's answers: a
// latency in milliseconds, or a hang as a timeout of zero, which passes
// nothing and closes nothing until the toxic is removed.
func toxicArgs(toxic labspec.Toxic) []string {
	attribute := "timeout=0"
	kind := "timeout"
	if toxic.Type == labspec.ToxicLatency {
		kind = "latency"
		attribute = "latency=" + strconv.FormatInt(time.Duration(toxic.Latency).Milliseconds(), 10)
	}
	return []string{proxyCLI, "toxic", "add", "--downstream", "-t", kind, "-n", toxicName, "-a", attribute, toxic.Victim}
}
