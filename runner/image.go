package main

import (
	"context"
	"fmt"
	"path/filepath"
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

// treeLabel is where scripts/build-enforcer.sh records the tree it verified the
// image's source against. Nothing else in the build sets it, so an image built
// with the pinned build arguments by any other route carries none, unless it
// sets the label by hand or builds FROM an image that carries it.
const treeLabel = "io.guardana.playground.enforcer.tree"

// readImages records which image the run's enforcer container ran and which
// image the pin's tag names on this machine, with the commit it was built from.
func (l lab) readImages(ctx context.Context, compose Compose, profiles []string, plane *check.Plane) {
	var problems []string
	running, err := compose.ContainerImage(ctx, profiles, enforcerService)
	if err != nil {
		problems = append(problems, "the enforcer container: "+err.Error())
	}
	plane.RunningImage = running
	problems = append(problems, l.readTree(ctx, plane)...)
	switch {
	case l.development != nil:
		plane.PinnedImage, plane.PinnedLabel = l.development.ID, l.development.Version
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

// readTree records the tree versions.env pins the enforcer's source at and the
// tree label of the image the run's enforcer container runs.
func (l lab) readTree(ctx context.Context, plane *check.Plane) []string {
	var problems []string
	pins, err := readPins(filepath.Join(l.root, versionFile))
	if err != nil {
		problems = append(problems, "the pins cannot be read: "+err.Error())
	}
	plane.TreePin = pinValue(pins, "ENFORCER_TREE")
	if l.development != nil {
		plane.TreePin = l.development.Tree
	}
	if plane.RunningImage == "" || l.inspect == nil {
		return problems
	}
	format := fmt.Sprintf(`{{index .Config.Labels %q}}`, treeLabel)
	tree, err := l.inspect(ctx, "docker", "image", "inspect", "--format", format, plane.RunningImage)
	if err != nil {
		return append(problems, "the running image's tree label: "+err.Error())
	}
	if tree != "<no value>" {
		plane.RunningTree = tree
	}
	return problems
}
