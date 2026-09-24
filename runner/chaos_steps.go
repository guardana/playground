package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

// addToxic puts the toxic on the victim's proxy and reads the proxy's own
// account of that proxy back: the toxic is held only when the proxy lists it
// with the attribute the scenario asked for.
func (l lab) addToxic(
	ctx context.Context, compose Compose, profiles []string, toxic labspec.Toxic, fault *check.ChaosFault, log *strings.Builder,
) {
	args := toxicArgs(toxic)
	added, err := compose.Exec(ctx, profiles, proxyService, args)
	fault.Applied = err == nil && added.ExitCode == 0
	fmt.Fprintf(log, "toxic added on %s: exit %d %v %s\n", toxic.Victim, added.ExitCode, err, strings.TrimSpace(added.Stdout+added.Stderr))
	listed, _ := l.inspectProxy(ctx, compose, profiles, toxic.Victim, log)
	fault.Held = fault.Applied && strings.Contains(listed, toxicName+"\ttype=") && strings.Contains(listed, "\t"+args[len(args)-2]+"\t")
	noteRead(fault, "the proxy listed %q", strings.TrimSpace(listed))
}

// removeToxic lifts the toxic, and it is lifted only when the proxy no longer
// lists it.
func (l lab) removeToxic(
	ctx context.Context, compose Compose, profiles []string, victim string, fault *check.ChaosFault, log *strings.Builder,
) {
	removed, err := compose.Exec(ctx, profiles, proxyService, []string{proxyCLI, "toxic", "remove", "-n", toxicName, victim})
	fmt.Fprintf(log, "toxic removed from %s: exit %d %v %s\n", victim, removed.ExitCode, err, strings.TrimSpace(removed.Stdout+removed.Stderr))
	listed, read := l.inspectProxy(ctx, compose, profiles, victim, log)
	fault.Lifted = err == nil && removed.ExitCode == 0 && read && !strings.Contains(listed, toxicName)
	if read {
		noteRead(fault, "after the removal it listed %q", strings.TrimSpace(listed))
	} else {
		noteRead(fault, "after the removal it could not be read")
	}
}

// inspectProxy is the proxy's listing of one proxy's toxics, and whether it
// could be read at all: a proxy with no toxic lists nothing.
func (l lab) inspectProxy(
	ctx context.Context, compose Compose, profiles []string, victim string, log *strings.Builder,
) (string, bool) {
	listed, err := compose.Exec(ctx, profiles, proxyService, []string{proxyCLI, "inspect", victim})
	fmt.Fprintf(log, "proxy %s: exit %d %v\n%s\n", victim, listed.ExitCode, err, strings.TrimSpace(listed.Stdout))
	if err != nil || listed.ExitCode != 0 {
		return "", false
	}
	return listed.Stdout, true
}

// restartCollector reads the enforcer's /healthz while the collector is still
// down, which has to show records waiting for it, then starts it again. The
// collector is lifted when compose reports it running and the enforcer's
// exporter has had records acknowledged since it was down.
func (l lab) restartCollector(ctx context.Context, compose Compose, profiles []string, fault *check.ChaosFault, log *strings.Builder) {
	body, err := l.planeGet(ctx, compose, profiles, "/healthz")
	fmt.Fprintf(log, "healthz while the collector was down: %v %s\n", err, strings.TrimSpace(string(body)))
	var down planeHealth
	if err == nil {
		down, err = readHealth(body)
	}
	before, counted := down.acknowledged()
	switch {
	case err != nil:
		noteRead(fault, "/healthz while the collector was down: %v", err)
	case down.unacknowledged() > 0:
		fault.Held = fault.Applied
		noteRead(fault, "%d bytes unacknowledged while the collector was down", down.unacknowledged())
	default:
		noteRead(fault, "nothing unacknowledged while the collector was down")
	}
	err = compose.Up(ctx, profiles, []string{collectorService})
	fmt.Fprintf(log, "collector started: %v\n", err)
	running := err == nil && l.collectorRunning(ctx, compose, profiles, fault)
	if !counted {
		noteRead(fault, "no exporter.acknowledged while it was down to compare against")
		return
	}
	fault.Lifted = running && l.acknowledgedSince(ctx, compose, profiles, before, fault, log)
}

// collectorRunning is compose's own account of the collector's container.
func (l lab) collectorRunning(ctx context.Context, compose Compose, profiles []string, fault *check.ChaosFault) bool {
	services, err := compose.Status(ctx, profiles, []string{collectorService})
	for _, service := range services {
		if service.Name == collectorService {
			noteRead(fault, "compose reports the collector %s", service.Detail)
			return service.Running
		}
	}
	noteRead(fault, "compose reported nothing on the collector: %v", err)
	return false
}

// acknowledgedSince polls the enforcer's /healthz, within the drain's bound,
// until its exporter has had more records acknowledged than before.
func (l lab) acknowledgedSince(
	ctx context.Context, compose Compose, profiles []string, before int64, fault *check.ChaosFault, log *strings.Builder,
) bool {
	bound := l.drainBound
	if bound <= 0 {
		bound = drainTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	var last string
	for {
		body, err := l.planeGet(ctx, compose, profiles, "/healthz")
		health, readErr := readHealth(body)
		now, counted := health.acknowledged()
		switch {
		case err != nil || readErr != nil:
			last = fmt.Sprint(errors.Join(err, readErr))
		case counted && now > before:
			fmt.Fprintf(log, "healthz after the collector started: %s\n", strings.TrimSpace(string(body)))
			noteRead(fault, "the enforcer's exporter acknowledged %d records, %d while the collector was down", now, before)
			return true
		default:
			last = fmt.Sprintf("%d acknowledged", now)
		}
		select {
		case <-ctx.Done():
			noteRead(fault, "no record acknowledged since the collector was down: %s when the wait ended", last)
			return false
		case <-time.After(drainPoll):
		}
	}
}

// relist has the victim list its own tools through its own listener, as a
// client the enforcer knows nothing of. It happened when that listing
// describes a tool otherwise than the snapshot the enforcer's classification
// was pinned to.
func (l lab) relist(ctx context.Context, compose Compose, profiles []string, victim string, fault *check.ChaosFault, log *strings.Builder) {
	listed, err := compose.Exec(ctx, profiles, victim,
		[]string{"/relist", "http://127.0.0.1:" + servicePort + "/mcp"})
	fmt.Fprintf(log, "%s listed again: exit %d %v\n%s\n%s\n", victim, listed.ExitCode, err,
		strings.TrimSpace(listed.Stdout), strings.TrimSpace(listed.Stderr))
	fault.Applied = err == nil && listed.ExitCode == 0
	changed, err := l.describedAnew(victim, listed.Stdout)
	switch {
	case err != nil:
		noteRead(fault, "%v", err)
	case len(changed) == 0:
		noteRead(fault, "the second listing described every tool as %s/%s.json does", listingSnapshots, victim)
	default:
		fault.Held = fault.Applied
		noteRead(fault, "%s", strings.Join(changed, "; "))
	}
}

// describedAnew names each tool the printed listing describes otherwise than
// the victim's listing snapshot, or does not name at all.
func (l lab) describedAnew(victim, printed string) ([]string, error) {
	path := filepath.Join(l.root, listingSnapshots, victim+".json")
	body, err := os.ReadFile(path) // #nosec G304 -- the lab's own snapshot of a victim it names.
	if err != nil {
		return nil, err
	}
	var snapshot []listedTool
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	described := map[string]string{}
	for _, tool := range snapshot {
		described[tool.Name] = tool.Description
	}
	var changed []string
	for _, line := range strings.Split(strings.TrimSpace(printed), "\n") {
		var tool listedTool
		if json.Unmarshal([]byte(line), &tool) != nil || tool.Name == "" {
			continue
		}
		if was, known := described[tool.Name]; !known || was != tool.Description {
			changed = append(changed, fmt.Sprintf("%s described as %q, the snapshot %q", tool.Name, tool.Description, was))
		}
	}
	return changed, nil
}

// listedTool is what a listing snapshot and a printed listing both say of a tool.
type listedTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
