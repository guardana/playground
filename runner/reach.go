package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
	"github.com/guardana/playground/runner/dockercompose"
	"github.com/guardana/playground/runner/driver"
)

const (
	// outsideAddress is a documentation address (RFC 5737). It is dialled only
	// to read the error: a network with no route out answers that at once, and
	// the address leads nowhere should the lab ever have a route.
	outsideAddress = "192.0.2.1:443"
	// reachScript dials one host:port from the prober's network position and
	// prints the markers readProbe reads. Unreachable is claimed only for no
	// name or no route; anything else says nothing about the topology.
	reachScript = `import errno, socket, sys
target = sys.argv[1]
host, port = target.rsplit(":", 1)
try:
    socket.create_connection((host, int(port)), 3).close()
    print("probe reached", target)
except socket.gaierror as e:
    print("probe unreachable", target + ":", e)
except OSError as e:
    if e.errno not in (errno.ENETUNREACH, errno.EHOSTUNREACH):
        print("the dial told nothing:", e)
        sys.exit(2)
    print("probe unreachable", target + ":", e)
`
	// proxyTarget is what proxyScript names in the probe markers.
	proxyTarget = "a proxy"
	// proxyScript reads the prober's own environment: reached when any
	// *_proxy variable is set, which would carry its requests and make the
	// servers it probes no longer local to it. no_proxy names exceptions, not a
	// proxy.
	proxyScript = `import os
found = sorted(k for k, v in os.environ.items() if k.lower().endswith("_proxy") and k.lower() != "no_proxy" and v)
if found:
    print("probe reached a proxy:", ", ".join(found))
else:
    print("probe unreachable a proxy: none in the environment")
`
	// routeTarget is what routeScript names in the probe markers.
	routeTarget = "a default route"
	// routeScript reads the prober's own routing tables: reached when a
	// usable default route is there, unreachable when none is. A reject route
	// is the kernel's "no route", not a way out, and a kernel without
	// ipv6_route has no IPv6 route at all.
	routeScript = `import sys
UP, REJECT = 0x1, 0x200
def usable(flags):
    return flags & UP and not flags & REJECT
found = []
try:
    with open("/proc/net/route") as f:
        for c in (line.split() for line in f.read().splitlines()[1:]):
            if len(c) > 7 and c[1] == "00000000" and c[7] == "00000000" and usable(int(c[3], 16)):
                found.append("ipv4 via " + c[0])
    try:
        with open("/proc/net/ipv6_route") as f:
            for c in (line.split() for line in f.read().splitlines()):
                if len(c) > 9 and c[0] == "0" * 32 and c[1] == "00" and usable(int(c[8], 16)):
                    found.append("ipv6 via " + c[9])
    except FileNotFoundError:
        pass
except (OSError, ValueError) as e:
    print("the routing tables told nothing:", e)
    sys.exit(2)
if found:
    print("probe reached a default route:", ", ".join(found))
else:
    print("probe unreachable a default route: none in /proc/net/route or /proc/net/ipv6_route")
`
)

// reachOut dials each server the scenario probes and the outside address from
// inside a prober's service, reads its routing tables and its environment,
// and records all of it in probes.log.
func (l lab) reachOut(spec labspec.Scenario, runDir string) driver.ReachOut {
	return func(ctx context.Context, compose dockercompose.Compose, service string) check.VerifierReach {
		reach := check.VerifierReach{Source: filepath.Join(runDir, "probes.log")}
		var recorded strings.Builder
		record := func(what string, probe check.Probe) {
			fmt.Fprintf(&recorded, "%s %s ran=%t reached=%t %s\n", what, probe.Target, probe.Ran, probe.Reached, probe.Detail)
		}
		for _, server := range spec.Probed() {
			probe := l.python(ctx, compose, spec.Profile, service, server+":"+servicePort, reachScript, server+":"+servicePort)
			reach.Servers = append(reach.Servers, probe)
			record("server", probe)
		}
		reach.Outside = l.python(ctx, compose, spec.Profile, service, outsideAddress, reachScript, outsideAddress)
		record("outside", reach.Outside)
		reach.Routes = l.python(ctx, compose, spec.Profile, service, routeTarget, routeScript)
		record("routes", reach.Routes)
		reach.Proxy = l.python(ctx, compose, spec.Profile, service, proxyTarget, proxyScript)
		record("proxy", reach.Proxy)
		if err := writeBytes(reach.Source, []byte(recorded.String())); err != nil {
			l.note("writing the probe record: %v", err)
		}
		return reach
	}
}

// python runs one script in a prober's service and reads its markers.
func (l lab) python(ctx context.Context, compose dockercompose.Compose, profiles []string, service, target, script string, args ...string) check.Probe {
	split, err := compose.RunSplit(ctx, profiles, service, "python", append([]string{"-c", script}, args...))
	if err != nil {
		return check.Probe{Target: target, Detail: err.Error()}
	}
	return readProbe(target, split.Stdout)
}
