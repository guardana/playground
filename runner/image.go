package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/guardana/playground/runner/check"
)

// ContainerImage is the image ID the project's own container of service runs.
func (d dockerCompose) ContainerImage(ctx context.Context, profiles []string, service string) (string, error) {
	listed, err := d.capture(ctx, profiles, "ps", "--all", "-q", service)
	if err != nil {
		return "", err
	}
	ids := strings.Fields(listed)
	if len(ids) != 1 {
		return "", fmt.Errorf("the project has %d %s containers, want one", len(ids), service)
	}
	return command(ctx, "docker", "inspect", "--format", "{{.Image}}", ids[0])
}

// readImages records which image the run's enforcer container ran and which
// image the pin's tag names on this machine, with the commit it was built from.
func (l lab) readImages(ctx context.Context, compose Compose, profiles []string, plane *check.Plane) {
	var problems []string
	running, err := compose.ContainerImage(ctx, profiles, enforcerService)
	if err != nil {
		problems = append(problems, "the enforcer container: "+err.Error())
	}
	plane.RunningImage = running
	switch {
	case l.inspect == nil || l.enforcerImage == "":
		problems = append(problems, "no pinned image to compare with")
	default:
		at := strings.LastIndex(l.enforcerImage, ":")
		name, pin := l.enforcerImage[:max(at, 0)], l.enforcerImage[at+1:]
		pinned := inspectImage(ctx, l.inspect, name, pin, revisionLabel)
		plane.PinnedImage, plane.PinnedLabel = pinned.ID, pinned.Label
		if pinned.Missing != "" {
			problems = append(problems, l.enforcerImage+": "+pinned.Missing)
		}
	}
	plane.ImageDetail = strings.Join(problems, "; ")
}
