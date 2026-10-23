package control

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guardana/playground/runner/driver"
)

const (
	devTree    = "7540e1f1a77892946d11c4734ffe26a9cc7ae6c0"
	devVersion = "dev-7540e1f1a778"
	devRef     = "lab-enforcer-dev:7540e1f1a778"
	testPin    = "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"
)

// inspectDevelopment answers docker the way a development image built by
// scripts/build-enforcer-dev.sh would, with labels the test chooses.
func inspectDevelopment(labels string) driver.Lookup {
	return func(_ context.Context, _ string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "{{json .Config.Labels}}"):
			return labels, nil
		case strings.Contains(joined, treeLabel):
			return devTree, nil
		case strings.Contains(joined, revisionLabel):
			return "sha256:dd " + devVersion, nil
		}
		return "sha256:dd", nil
	}
}

func devLabels(source, version string) string {
	return `{"io.guardana.playground.enforcer.source":"` + source + `",` +
		`"io.guardana.playground.enforcer.tree":"` + devTree + `",` +
		`"org.opencontainers.image.revision":"` + version + `",` +
		`"io.guardana.playground.enforcer.source.path":"/work/control",` +
		`"io.guardana.playground.enforcer.source.head":"1415392",` +
		`"io.guardana.playground.enforcer.source.state":"dirty"}`
}

func TestADevelopmentImageIsReadFromItsOwnLabelsAndRefusedOtherwise(t *testing.T) {
	dev, err := readDevelopment(context.Background(), inspectDevelopment(devLabels("development", devVersion)), devRef)
	if err != nil {
		t.Fatalf("readDevelopment: %v", err)
	}
	if dev.Version != devVersion || dev.Tree != devTree || dev.State != "dirty" || dev.ID != "sha256:dd" {
		t.Errorf("read %+v", dev)
	}
	if line := dev.describe(testPin); !strings.Contains(line, "/work/control") || !strings.Contains(line, "not the pinned ENFORCER_COMMIT "+testPin) {
		t.Errorf("the build is described as %q", line)
	}
	refused := map[string]driver.Lookup{
		"a pinned build":       inspectDevelopment(devLabels("", testPin)),
		"another tree":         inspectDevelopment(devLabels("development", "dev-000000000000")),
		"labels that are none": inspectDevelopment("not json"),
		"no image": func(context.Context, string, ...string) (string, error) {
			return "", errors.New("No such image")
		},
	}
	for name, inspect := range refused {
		if _, err := readDevelopment(context.Background(), inspect, devRef); err == nil {
			t.Errorf("%s was taken for a development build", name)
		}
	}
}
