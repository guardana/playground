package main

import (
	"context"
	"errors"
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

func TestTheRunsOwnEnforcerContainerIsTheOneGraded(t *testing.T) {
	subject, compose, scenario := enforcerLab(t)
	var asked []string
	compose.image = func(service string) (string, error) {
		asked = append(asked, service)
		return "sha256:aa", nil
	}
	var inspected []string
	subject.inspect = func(_ context.Context, name string, args ...string) (string, error) {
		inspected = append([]string{name}, args...)
		return "sha256:aa " + testPin, nil
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
	if joined := strings.Join(inspected, " "); !strings.HasPrefix(joined, "docker image inspect") ||
		!strings.HasSuffix(joined, "lab-enforcer:"+testPin) {
		t.Errorf("the pinned image was read with %q", joined)
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
