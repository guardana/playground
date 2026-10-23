package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/guardana/playground/runner/report"
)

// InspectImage reads the local image name:pin, its ID and the value of label.
func InspectImage(ctx context.Context, run Lookup, name, pin, label string) report.Image {
	image := report.Image{Ref: name + ":" + pin, Want: pin}
	if name == "" || pin == "" {
		image.Missing = "versions.env names no image or no pin for it"
		return image
	}
	format := fmt.Sprintf(`{{.Id}} {{index .Config.Labels %q}}`, label)
	found, err := run(ctx, "docker", "image", "inspect", "--format", format, image.Ref)
	switch {
	case err != nil && strings.Contains(err.Error(), "No such image"):
		image.Missing = "not built on this machine"
		return image
	case err != nil:
		image.Missing = "unreadable: " + err.Error()
		return image
	}
	image.ID, image.Label, _ = strings.Cut(found, " ")
	if image.Label == "<no value>" {
		image.Label = ""
	}
	return image
}
