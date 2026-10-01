package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// Plane grades what the runner read from the enforcer itself: that the build
// serving the run reports the pinned commit and runs the image built from it,
// and that its spool drained into the collector before the trail was read. A trail read before the drain could
// be one the plane had not finished handing over.
type Plane struct {
	Pin string
	// Builder is the script whose build labels the image with its tree:
	// scripts/build-enforcer.sh at the pin, or the development build's.
	Builder     string
	Version     string
	VersionRead bool
	Drained     bool
	DrainDetail string
	Source      string
	// RunningImage is the image ID of the run's own enforcer container;
	// PinnedImage and PinnedLabel are the ID and revision label of the image
	// tagged with the pin. ImageDetail says why any of them was not read.
	RunningImage string
	PinnedImage  string
	PinnedLabel  string
	ImageDetail  string
	// TreePin is ENFORCER_TREE; RunningTree is the tree label of the image the
	// container runs, which only scripts/build-enforcer.sh sets, after it
	// checked the source against that tree.
	TreePin     string
	RunningTree string
}

// ID names the check in a report.
func (Plane) ID() string { return "plane" }

// Run reports the version and the drain.
func (p Plane) Run(_ context.Context, _ assertion.Records) ([]assertion.Result, error) {
	version := assertion.Result{
		Check:  "plane/version",
		Want:   "the enforcer reports commit " + p.Pin,
		Source: p.Source,
	}
	switch {
	case !p.VersionRead:
		version.Outcome, version.Got = assertion.Indeterminate, "its /brand could not be read"
	case p.Version == p.Pin && p.Pin != "":
		version.Outcome, version.Got = assertion.Pass, p.Version
	default:
		version.Outcome, version.Got = assertion.Fail, spoken(p.Version)
		version.Detail = fmt.Sprintf("the run's enforcer serves %s", spoken(p.Version))
	}
	drained := assertion.Result{
		Check: "plane/drained",
		Want: "nothing unacknowledged in the spool and nothing quarantined, truncated, refused or dropped on the way, " +
			"and the collector stopped so its file is flushed, before the trail is read",
		Got:    spoken(p.DrainDetail),
		Source: p.Source,
	}
	if p.Drained {
		drained.Outcome = assertion.Pass
	} else {
		drained.Outcome = assertion.Fail
	}
	return []assertion.Result{version, p.image(), drained}, nil
}

// image grades the image the run's enforcer container ran against the image
// tagged with the pin, the commit that image was built from, and the tree the
// build script verified its source against.
func (p Plane) image() assertion.Result {
	result := assertion.Result{
		Check: "plane/image",
		Want: "the run's enforcer container runs the image tagged with the pin, built from commit " + p.Pin +
			" and labelled by " + p.builder() + " with tree " + spoken(p.TreePin),
		Got:    p.RunningImage,
		Source: p.Source,
		Detail: p.ImageDetail,
	}
	switch {
	case p.RunningImage == "" || p.PinnedImage == "":
		result.Outcome, result.Got = assertion.Fail, "the container's image or the pinned image was not read"
	case p.Pin == "" || p.PinnedLabel != p.Pin:
		result.Outcome = assertion.Fail
		result.Detail = "the image tagged with the pin was built from " + spoken(p.PinnedLabel)
	case p.RunningImage != p.PinnedImage:
		result.Outcome = assertion.Fail
		result.Detail = "the image tagged with the pin is " + p.PinnedImage
	case p.TreePin == "":
		result.Outcome = assertion.Fail
		result.Detail = strings.TrimSuffix("versions.env pins no ENFORCER_TREE; "+p.ImageDetail, "; ")
	case p.RunningTree == "" && p.ImageDetail != "":
		result.Outcome = assertion.Fail
		result.Detail = "the running image's tree label was not read: " + p.ImageDetail
	case p.RunningTree != p.TreePin:
		result.Outcome = assertion.Fail
		result.Detail = "the running image carries " + treeFound(p.RunningTree) + ", so " + p.builder() + " did not build it"
	default:
		result.Outcome = assertion.Pass
	}
	return result
}

func (p Plane) builder() string {
	if p.Builder == "" {
		return "scripts/build-enforcer.sh"
	}
	return p.Builder
}

func treeFound(tree string) string {
	if tree == "" {
		return "no tree label"
	}
	return "tree label " + tree
}
