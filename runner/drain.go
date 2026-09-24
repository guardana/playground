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

	"github.com/guardana/playground/runner/check"
)

// How the runner waits for the enforcer's trail to reach the collector.
const (
	enforcerService  = "enforcer"
	collectorOutput  = "collector/otlp-logs.json"
	collectorService = "collector"
	drainTimeout     = 90 * time.Second
	drainPoll        = 250 * time.Millisecond
)

var errNotDrained = errors.New("the spool still holds unacknowledged records")

// drainPlane reads the enforcer's version, waits until its spool holds nothing
// unacknowledged and lost nothing on the way, stops the collector so its file
// exporter flushes, and writes the run's evidence.jsonl from that file. Every
// step is recorded in plane.log.
func (l lab) drainPlane(ctx context.Context, compose Compose, profiles []string, pin, runDir string) check.Plane {
	bound := l.drainBound
	if bound <= 0 {
		bound = drainTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	plane := check.Plane{Pin: pin, Source: filepath.Join(runDir, "plane.log")}
	var log strings.Builder
	defer func() { _ = os.WriteFile(plane.Source, []byte(log.String()), 0o600) }() // #nosec G703 -- inside the run directory.

	if body, err := l.planeGet(ctx, compose, profiles, "/brand"); err == nil {
		var brand struct {
			Version string `json:"version"`
		}
		plane.VersionRead = json.Unmarshal(body, &brand) == nil
		plane.Version = brand.Version
		fmt.Fprintf(&log, "brand %s\n", body)
	} else {
		fmt.Fprintf(&log, "brand unreadable: %v\n", err)
	}
	l.readImages(ctx, compose, profiles, &plane)
	fmt.Fprintf(&log, "image running %s, pinned %s built from %s: %s\n",
		plane.RunningImage, plane.PinnedImage, plane.PinnedLabel, plane.ImageDetail)
	health, err := l.waitHandedOver(ctx, compose, profiles)
	fmt.Fprintf(&log, "healthz %s\nhanded over: %v\n", strings.TrimSpace(health), err)
	if err == nil {
		// The collector acknowledges a delivery before its file exporter has
		// written it; stopping it is what flushes the file.
		err = compose.Stop(ctx, profiles, collectorService)
		fmt.Fprintf(&log, "collector stopped: %v\n", err)
	}
	if err == nil {
		err = writeEvidenceFromCollector(filepath.Join(runDir, collectorOutput),
			filepath.Join(runDir, "evidence.jsonl"), l.namespace, maxEvents)
		fmt.Fprintf(&log, "evidence written: %v\n", err)
	}
	plane.Drained = err == nil
	plane.DrainDetail = "drained"
	if err != nil {
		plane.DrainDetail = err.Error()
	}
	return plane
}

// waitHandedOver polls /healthz until the spool holds nothing
// unacknowledged, then requires that nothing was lost on the way. A lossy or
// unreadable answer ends the wait at once: a quarantine does not drain.
func (l lab) waitHandedOver(ctx context.Context, compose Compose, profiles []string) (string, error) {
	var last string
	for {
		body, err := l.planeGet(ctx, compose, profiles, "/healthz")
		if err != nil {
			last = err.Error()
		} else {
			health, err := readHealth(body)
			switch {
			case err != nil:
				return string(body), err
			case health.unacknowledged() == 0:
				return string(body), health.lost()
			}
			last = fmt.Sprintf("%d bytes", health.unacknowledged())
		}
		select {
		case <-ctx.Done():
			return last, fmt.Errorf("%w: %s when the wait ended", errNotDrained, last)
		case <-time.After(drainPoll):
		}
	}
}

// planeGet reads one of the enforcer's health endpoints from inside its own
// container, whatever the status: a 503 is an answer worth reading.
func (l lab) planeGet(ctx context.Context, compose Compose, profiles []string, path string) ([]byte, error) {
	split, err := compose.Exec(ctx, profiles, enforcerService,
		[]string{"/healthprobe", "-show", "http://127.0.0.1:8081" + path})
	if err != nil {
		return nil, err
	}
	if split.ExitCode != 0 {
		return nil, fmt.Errorf("reading %s exited %d: %s", path, split.ExitCode, strings.TrimSpace(split.Stderr))
	}
	status, body, found := strings.Cut(split.Stdout, "\n")
	if !found || !strings.HasPrefix(status, "status ") {
		return nil, fmt.Errorf("reading %s printed no status line", path)
	}
	return []byte(body), nil
}
