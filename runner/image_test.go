package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/assertion"
)

func (f *fakeCompose) ContainerImage(ctx context.Context, _ []string, service string) (string, error) {
	if err := f.called(ctx, "container image"); err != nil {
		return "", err
	}
	if f.image == nil {
		return "", errors.New("no container")
	}
	return f.image(service)
}

// testTree is the tree of testPin in the enforcer's repository.
const testTree = "4c1159f7c7142e0eafb56cdf1bca17b9dd5fe890"

// inspectEnforcer answers docker image inspect for the enforcer image sha256:aa
// built from testPin, with tree as its tree label only when the image is asked
// for by its id, the way the run's own container names it.
func inspectEnforcer(tree string) lookup {
	return func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "io.guardana.playground.enforcer.tree") {
			if args[len(args)-1] == "sha256:aa" {
				return tree, nil
			}
			return "", nil
		}
		return "sha256:aa " + testPin, nil
	}
}

func TestTheRunsOwnEnforcerContainerIsTheOneGraded(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	var asked []string
	compose.image = func(service string) (string, error) {
		asked = append(asked, service)
		return "sha256:aa", nil
	}
	var inspected []string
	answer := inspectEnforcer(testTree)
	subject.inspect = func(ctx context.Context, name string, args ...string) (string, error) {
		inspected = append(inspected, strings.Join(append([]string{name}, args...), " "))
		return answer(ctx, name, args...)
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result := results(graded)["plane/image"]; result.Outcome != assertion.Pass {
		t.Errorf("plane/image is %s: %+v", result.Outcome, result)
	}
	if !slices.Equal(asked, []string{"enforcer"}) {
		t.Errorf("the image was read from %v, want the enforcer's container", asked)
	}
	if !slices.ContainsFunc(inspected, func(call string) bool {
		return strings.HasPrefix(call, "docker image inspect") && strings.HasSuffix(call, "lab-enforcer:"+testPin)
	}) {
		t.Errorf("the pinned image was never read: %q", inspected)
	}
	if !slices.ContainsFunc(inspected, func(call string) bool {
		return strings.HasPrefix(call, "docker image inspect") &&
			strings.Contains(call, `"io.guardana.playground.enforcer.tree"`) && strings.HasSuffix(call, " sha256:aa")
	}) {
		t.Errorf("the running image's tree label was never read: %q", inspected)
	}
}

// The tree is read from the image the container runs and compared with
// versions.env: the tag, an image built by hand beside the script, or a pin
// that names no tree is not what the build script vouched for.
func TestAnEnforcerImageWithoutTheVerifiedTreeFails(t *testing.T) {
	for name, tc := range map[string]struct {
		set   func(*lab)
		found string
	}{
		"built by hand": {func(l *lab) { l.inspect = inspectEnforcer("") }, "no tree label"},
		"docker saying no value": {func(l *lab) { l.inspect = inspectEnforcer("<no value>") },
			"no tree label"},
		"another tree": {func(l *lab) { l.inspect = inspectEnforcer("5d2260") },
			"tree label 5d2260"},
		"the tree only on the tag": {func(l *lab) {
			l.inspect = func(_ context.Context, _ string, args ...string) (string, error) {
				if strings.Contains(strings.Join(args, " "), "enforcer.tree") {
					if args[len(args)-1] == "sha256:aa" {
						return "", nil
					}
					return testTree, nil
				}
				return "sha256:aa " + testPin, nil
			}
		}, "no tree label"},
		"no tree pinned": {func(l *lab) {
			writeFile(filepath.Join(l.root, versionFile), "ENFORCER_COMMIT="+testPin+"\n")
		}, "versions.env pins no ENFORCER_TREE"},
		"the label unreadable": {func(l *lab) {
			l.inspect = func(_ context.Context, _ string, args ...string) (string, error) {
				if strings.Contains(strings.Join(args, " "), "enforcer.tree") {
					return "", errors.New("exit status 1: daemon gone")
				}
				return "sha256:aa " + testPin, nil
			}
		}, "daemon gone"},
	} {
		t.Run(name, func(t *testing.T) {
			subject, _, scenario := enforcerLab(t)
			tc.set(&subject)
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			result := results(graded)["plane/image"]
			if result.Outcome != assertion.Fail || !strings.Contains(result.Detail, tc.found) {
				t.Errorf("plane/image is %s, want a failure saying %q: %+v", result.Outcome, tc.found, result)
			}
			if name == "another tree" {
				// The record the check cites has to show both trees it compared.
				logged, err := os.ReadFile(result.Source)
				if err != nil || !strings.Contains(string(logged), `tree label "5d2260", ENFORCER_TREE "`+testTree+`"`) {
					t.Errorf("%s does not show the trees compared (%v):\n%s", result.Source, err, logged)
				}
			}
		})
	}
}

func TestAnEnforcerContainerRunningAnotherImageFails(t *testing.T) {
	for name, set := range map[string]func(*lab, *fakeCompose){
		"another image": func(_ *lab, c *fakeCompose) {
			c.image = func(string) (string, error) { return "sha256:bb", nil }
		},
		"no container": func(_ *lab, c *fakeCompose) { c.image = nil },
		"the pinned tag from another commit": func(l *lab, _ *fakeCompose) {
			l.inspect = func(context.Context, string, ...string) (string, error) { return "sha256:aa 0000", nil }
		},
		"the pinned tag absent": func(l *lab, _ *fakeCompose) {
			l.inspect = func(context.Context, string, ...string) (string, error) {
				return "", errors.New("No such image")
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			subject, compose, scenario := enforcerLab(t)
			set(&subject, compose)
			graded, err := subject.execute(context.Background(), scenario)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if result := results(graded)["plane/image"]; result.Outcome != assertion.Fail {
				t.Errorf("plane/image is %s: %+v", result.Outcome, result)
			}
		})
	}
}
