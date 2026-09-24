package check_test

import (
	"context"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

const (
	pin  = "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"
	tree = "4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890"
)

func imageResult(t *testing.T, plane check.Plane) assertion.Result {
	t.Helper()
	results, err := plane.Run(context.Background(), assertion.Records{})
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Check == "plane/image" {
			return result
		}
	}
	t.Fatal("no plane/image result")
	return assertion.Result{}
}

// The container the run used is the image tagged with the pin, and that image
// was built from the pin: a tag moved by hand or a container left from another
// build is not the system under test.
func TestTheEnforcerContainerRunsThePinnedImage(t *testing.T) {
	good := check.Plane{Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa", PinnedLabel: pin,
		TreePin: tree, RunningTree: tree}
	if result := imageResult(t, good); result.Outcome != assertion.Pass {
		t.Errorf("the pinned image was %s: %+v", result.Outcome, result)
	}
	for name, plane := range map[string]check.Plane{
		"another image running":     {Pin: pin, RunningImage: "sha256:bb", PinnedImage: "sha256:aa", PinnedLabel: pin, TreePin: tree, RunningTree: tree},
		"the tag built elsewhere":   {Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa", PinnedLabel: "0000", TreePin: tree, RunningTree: tree},
		"no label on the tag":       {Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa", TreePin: tree, RunningTree: tree},
		"the container unread":      {Pin: pin, PinnedImage: "sha256:aa", PinnedLabel: pin, TreePin: tree, ImageDetail: "no container"},
		"the pinned image unread":   {Pin: pin, RunningImage: "sha256:aa", PinnedLabel: pin, TreePin: tree, RunningTree: tree},
		"no pin and nothing at all": {},
	} {
		if result := imageResult(t, plane); result.Outcome != assertion.Fail {
			t.Errorf("%s was %s: %+v", name, result.Outcome, result)
		}
	}
}

// Only scripts/build-enforcer.sh verifies that the image's source is the
// commit's own tree, and only it labels the image with that tree: an image built
// with the pinned build argument by any other route carries the same revision
// stamps and no tree.
func TestAnEnforcerImageTheBuildScriptDidNotVouchForFails(t *testing.T) {
	for name, tc := range map[string]struct {
		plane check.Plane
		found string
	}{
		"no tree label":  {check.Plane{TreePin: tree}, "no tree label"},
		"another tree":   {check.Plane{TreePin: tree, RunningTree: "5d2260"}, "5d2260"},
		"no tree pinned": {check.Plane{RunningTree: tree}, "versions.env pins no ENFORCER_TREE"},
		"the label unread": {check.Plane{TreePin: tree, ImageDetail: "the running image's tree label: daemon gone"},
			"not read: the running image's tree label: daemon gone"},
		"neither tree": {check.Plane{}, "versions.env pins no ENFORCER_TREE"},
	} {
		plane := tc.plane
		plane.Pin, plane.RunningImage, plane.PinnedImage, plane.PinnedLabel = pin, "sha256:aa", "sha256:aa", pin
		result := imageResult(t, plane)
		if result.Outcome != assertion.Fail {
			t.Errorf("%s was %s: %+v", name, result.Outcome, result)
		}
		if !strings.Contains(result.Detail, tc.found) {
			t.Errorf("%s: the detail %q does not say %q", name, result.Detail, tc.found)
		}
	}
}
