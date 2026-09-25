package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// probes asks, from inside agent-net, whether the gateway answers and one
// victim does not. Both answers are recorded, in memory for the check and on
// disk for the person the check sends to a file. Neither is inferred from the
// other: a topology where nothing at all is reachable would otherwise look like
// a topology that is right about the victim.
func (l lab) probes(
	ctx context.Context, compose Compose, spec labspec.Scenario, trajectory labspec.Trajectory, runDir string,
) (check.Probe, check.Probe, []check.Probe, string) {
	gateway := l.probe(ctx, compose, spec.Profile, enforcerService+":"+servicePort)
	victim := l.probe(ctx, compose, spec.Profile, trajectory.Steps[0].Call.Server+":"+servicePort)
	proxies, unnamed := l.sealedProxies(spec)
	var sealed []check.Probe
	for _, target := range append(sealedFromAgent(spec), proxies...) {
		sealed = append(sealed, l.probe(ctx, compose, spec.Profile, target))
	}
	if unnamed != nil {
		sealed = append(sealed, check.Probe{Target: proxyService, Detail: unnamed.Error()})
	}

	source := filepath.Join(runDir, "probes.log")
	recorded := fmt.Sprintf("gateway %s ran=%t reached=%t %s\nvictim %s ran=%t reached=%t %s\n",
		gateway.Target, gateway.Ran, gateway.Reached, gateway.Detail,
		victim.Target, victim.Ran, victim.Reached, victim.Detail)
	for _, probe := range sealed {
		recorded += fmt.Sprintf("sealed %s ran=%t reached=%t %s\n", probe.Target, probe.Ran, probe.Reached, probe.Detail)
	}
	// #nosec G703 -- the path is inside the run directory the runner made.
	if err := os.WriteFile(source, []byte(recorded), 0o600); err != nil {
		l.note("writing the probe record: %v", err)
	}
	return gateway, victim, sealed, source
}

func (l lab) probe(ctx context.Context, compose Compose, profiles []string, target string) check.Probe {
	execution, err := compose.RunOnce(ctx, profiles, agentService, []string{"-probe", target})
	if err != nil {
		return check.Probe{Target: target, Detail: err.Error()}
	}
	return readProbe(target, execution.Output)
}
