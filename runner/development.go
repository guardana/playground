package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// The labels scripts/build-enforcer-dev.sh sets on a development image.
const (
	sourceLabel      = "io.guardana.playground.enforcer.source"
	sourcePathLabel  = "io.guardana.playground.enforcer.source.path"
	sourceHeadLabel  = "io.guardana.playground.enforcer.source.head"
	sourceStateLabel = "io.guardana.playground.enforcer.source.state"
	developmentValue = "development"
)

// devBuild is a development image of the enforcer, built from a checkout's
// working tree rather than from the pinned release. A run against one grades
// that tree, and every line and report it writes says so.
type devBuild struct {
	Ref, ID, Version, Tree, Path, Head, State string
}

// readDevelopment reads a development image by its reference and refuses one
// the development build did not make: no development label, no tree, or a
// version that is not the tree's.
func readDevelopment(ctx context.Context, inspect lookup, ref string) (devBuild, error) {
	ctx, cancel := context.WithTimeout(ctx, provenanceTimeout)
	defer cancel()
	id, err := inspect(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", ref)
	if err != nil {
		return devBuild{}, fmt.Errorf("the development image %s: %w", ref, err)
	}
	body, err := inspect(ctx, "docker", "image", "inspect", "--format", "{{json .Config.Labels}}", ref)
	if err != nil {
		return devBuild{}, fmt.Errorf("the development image %s: %w", ref, err)
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(body), &labels); err != nil {
		return devBuild{}, fmt.Errorf("the development image %s: its labels: %w", ref, err)
	}
	dev := devBuild{
		Ref: ref, ID: strings.TrimSpace(id), Version: labels[revisionLabel], Tree: labels[treeLabel],
		Path: labels[sourcePathLabel], Head: labels[sourceHeadLabel], State: labels[sourceStateLabel],
	}
	switch {
	case labels[sourceLabel] != developmentValue:
		return devBuild{}, fmt.Errorf("%s is not a development build (scripts/build-enforcer-dev.sh makes one)", ref)
	case len(dev.Tree) < 12 || dev.Version != "dev-"+dev.Tree[:12]:
		return devBuild{}, fmt.Errorf("%s reports version %q for tree %q, not the development build's", ref, dev.Version, dev.Tree)
	}
	return dev, nil
}

// describe is the development build in one line, for the report and the run's
// last line, with the pin the run did not use.
func (d devBuild) describe(pin string) string {
	return fmt.Sprintf("development build `%s` (%s) of %s at %s, %s, tree %s; not the pinned ENFORCER_COMMIT %s",
		d.Ref, or(d.ID, "no id"), or(d.Path, "an unnamed checkout"), or(d.Head, "an unknown commit"),
		or(d.State, "state unknown"), d.Tree, pin)
}
