package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

const (
	devTree    = "7540e1f1a77892946d11c4734ffe26a9cc7ae6c0"
	devVersion = "dev-7540e1f1a778"
	devRef     = "lab-enforcer-dev:7540e1f1a778"
)

// inspectDevelopment answers docker the way a development image built by
// scripts/build-enforcer-dev.sh would, with labels the test chooses.
func inspectDevelopment(labels string) lookup {
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
	refused := map[string]lookup{
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

// developmentLab is a lab pointed at a development image through
// withDevelopment, with docker answered by inspectDevelopment and every
// signing call recorded in signed.
func developmentLab(t *testing.T, serves string) (lab, *fakeCompose, string, *[]string) {
	t.Helper()
	subject, compose, scenario := enforcerLab(t)
	answer := inspectDevelopment(devLabels("development", devVersion))
	var signed []string
	docker := func(ctx context.Context, name string, args ...string) (string, error) {
		if slices.Contains(args, "keygen") {
			return "", writeProbeKey(args)
		}
		if len(args) > 0 && args[0] == "run" {
			signed = append(signed, strings.Join(args, " "))
			return "", writeBundle(args)
		}
		return answer(ctx, name, args...)
	}
	subject.inspect = docker
	subject, err := withDevelopment(context.Background(), subject, devRef, subject.workspace, docker)
	if err != nil {
		t.Fatal(err)
	}
	compose.image = func(string) (string, error) { return "sha256:dd", nil }
	brand := compose.exec
	compose.exec = func(service string, args []string) Split {
		if strings.HasSuffix(args[len(args)-1], "/brand") {
			return Split{Stdout: "status 200\n" + `{"version":"` + serves + `"}`}
		}
		return brand(service, args)
	}
	return subject, compose, scenario, &signed
}

// writeBundle stands in for the signing container: it writes the bundle into
// the directory mounted at /out.
func writeBundle(args []string) error {
	for i, arg := range args {
		if i > 0 && args[i-1] == "-v" && strings.HasSuffix(arg, ":/out") {
			return os.WriteFile(filepath.Join(strings.TrimSuffix(arg, ":/out"), bundleName), []byte("signed"), 0o600)
		}
	}
	return errors.New("no /out mount")
}

func TestADevelopmentRunIsCheckedAgainstTheBuildItNamesAndSaysSo(t *testing.T) {
	subject, compose, scenario, signed := developmentLab(t, devVersion)
	var out bytes.Buffer
	ran := runEach(context.Background(), subject, []string{scenario}, &out)
	if len(ran) != 1 || ran[0].outcome != assertion.Pass {
		t.Fatalf("the development run was %+v:\n%s", ran, out.String())
	}
	if !strings.Contains(out.String(), "dev-catalogue") {
		t.Errorf("the run's line does not say it ran a development build: %q", out.String())
	}
	if compose.env["LAB_ENFORCER_REF"] != devRef || compose.env["LAB_ENFORCER_TAG"] != "7540e1f1a778" {
		t.Errorf("compose was given LAB_ENFORCER_REF=%q LAB_ENFORCER_TAG=%q", compose.env["LAB_ENFORCER_REF"], compose.env["LAB_ENFORCER_TAG"])
	}
	if len(*signed) != 1 || !strings.Contains((*signed)[0], " "+devRef+" ") {
		t.Errorf("the policy was not signed with the development build: %q", *signed)
	}
	if built := subject.describe(context.Background()).Development; !strings.Contains(built, devRef) ||
		!strings.Contains(built, "not the pinned ENFORCER_COMMIT") {
		t.Errorf("the report's provenance does not name the development build: %q", built)
	}
}

func TestADevelopmentRunServedByThePinnedBinaryIsRed(t *testing.T) {
	subject, _, scenario, _ := developmentLab(t, testPin)
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/version"]; result.Outcome != assertion.Fail || result.Got != testPin {
		t.Errorf("an enforcer serving the pin in a development run was %s: %+v", result.Outcome, result)
	}
}

// The tag can be built again while a catalogue runs; the run is held to the
// image it read at its start.
func TestADevelopmentRunOnAnotherImageThanItReadIsRed(t *testing.T) {
	subject, compose, scenario, _ := developmentLab(t, devVersion)
	compose.image = func(string) (string, error) { return "sha256:ee", nil }
	read := subject.inspect
	subject.inspect = func(ctx context.Context, name string, args ...string) (string, error) {
		if answer, err := read(ctx, name, args...); strings.HasPrefix(answer, "sha256:dd") {
			return "sha256:ee" + strings.TrimPrefix(answer, "sha256:dd"), err
		}
		return read(ctx, name, args...)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/image"]; result.Outcome != assertion.Fail {
		t.Errorf("a container running another image than the one read was %s: %+v", result.Outcome, result)
	}
}

// The runner's own value wins over one the shell exports, so a pinned run
// cannot be pointed at another image from outside.
func TestTheEnforcerImageIsTheRunnersWhateverTheShellExports(t *testing.T) {
	t.Setenv("LAB_ENFORCER_REF", devRef)
	subject, _, _ := enforcerLab(t)
	env := subject.environment("flow-01-20260909T120000Z-0badc0de", t.TempDir())
	composed := dockerCompose{}.WithEnv(env).(dockerCompose)
	last := ""
	for _, entry := range composed.env {
		if value, found := strings.CutPrefix(entry, "LAB_ENFORCER_REF="); found {
			last = value
		}
	}
	if last != subject.enforcerImage || last != "lab-enforcer:"+testPin {
		t.Errorf("compose would run %q, want the pinned %q", last, subject.enforcerImage)
	}
}
