package check_test

import (
	"context"
	"testing"

	"github.com/guardana/playground/internal/assertion"
	"github.com/guardana/playground/runner/check"
)

const pin = "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"

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
	good := check.Plane{Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa", PinnedLabel: pin}
	if result := imageResult(t, good); result.Outcome != assertion.Pass {
		t.Errorf("the pinned image was %s: %+v", result.Outcome, result)
	}
	for name, plane := range map[string]check.Plane{
		"another image running":     {Pin: pin, RunningImage: "sha256:bb", PinnedImage: "sha256:aa", PinnedLabel: pin},
		"the tag built elsewhere":   {Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa", PinnedLabel: "0000"},
		"no label on the tag":       {Pin: pin, RunningImage: "sha256:aa", PinnedImage: "sha256:aa"},
		"the container unread":      {Pin: pin, PinnedImage: "sha256:aa", PinnedLabel: pin, ImageDetail: "no container"},
		"the pinned image unread":   {Pin: pin, RunningImage: "sha256:aa", PinnedLabel: pin},
		"no pin and nothing at all": {},
	} {
		if result := imageResult(t, plane); result.Outcome != assertion.Fail {
			t.Errorf("%s was %s: %+v", name, result.Outcome, result)
		}
	}
}
