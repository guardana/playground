package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/runner/report"
)

func TestReadPinsKeepsTheFileOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "versions.env")
	writeFile(path, "# comment\n\nB_PIN=two\nA_PIN=python:3.13-slim\nEMPTY=\n")
	pins, err := readPins(path)
	if err != nil {
		t.Fatalf("readPins: %v", err)
	}
	want := []report.Pin{{Name: "B_PIN", Value: "two"}, {Name: "A_PIN", Value: "python:3.13-slim"}, {Name: "EMPTY", Value: ""}}
	if !slices.Equal(pins, want) {
		t.Errorf("readPins = %v, want %v", pins, want)
	}
}

func TestReadPinsRefusesALineThatIsNotAPin(t *testing.T) {
	for _, body := range []string{"export A=1\n", "a=1\n", "A 1\n", "A=1\nB\n"} {
		path := filepath.Join(t.TempDir(), "versions.env")
		writeFile(path, body)
		if pins, err := readPins(path); err == nil {
			t.Errorf("readPins(%q) = %v, want a refusal", body, pins)
		}
	}
}

func TestTheReportCarriesWhatProducedTheRun(t *testing.T) {
	compose := workingCompose("")
	subject, scenario := labUnderTest(t, compose)
	compose.reports = subject.reports
	subject.describe = func(ctx context.Context) report.Provenance {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the provenance lookups were given no deadline")
		}
		return report.Provenance{Lab: "lab-commit-under-test"}
	}
	graded, err := subject.execute(context.Background(), scenario)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(subject.reports, graded.RunID, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "- Lab: lab-commit-under-test") {
		t.Errorf("report.md does not carry the provenance:\n%s", page)
	}
}

// dockerSays answers `docker image inspect` the way the daemon would, with the
// output or error given for every image.
func dockerSays(out string, err error) lookup {
	return func(_ context.Context, name string, args ...string) (string, error) {
		if name != "docker" || len(args) < 2 || args[0] != "image" || args[1] != "inspect" {
			return "", fmt.Errorf("unexpected lookup %s %v", name, args)
		}
		return out, err
	}
}

func TestInspectImageReadsWhatTheDaemonSays(t *testing.T) {
	commit := "e72ebe261af4ca9bb4b683ddedda81bfcc5de906"
	for _, c := range []struct {
		name        string
		out         string
		err         error
		wantLabel   string
		wantMissing string
	}{
		{name: "another commit's label", out: "sha256:aa 0123456789abcdef0123456789abcdef01234567",
			wantLabel: "0123456789abcdef0123456789abcdef01234567"},
		{name: "no label", out: "sha256:aa <no value>"},
		{name: "not built", err: errors.New("exit status 1: Error response from daemon: No such image: x:y"),
			wantMissing: "not built on this machine"},
		{name: "daemon down", err: errors.New("exit status 1: Cannot connect to the Docker daemon"),
			wantMissing: "unreadable: exit status 1: Cannot connect to the Docker daemon"},
	} {
		t.Run(c.name, func(t *testing.T) {
			image := inspectImage(context.Background(), dockerSays(c.out, c.err), "playground-enforcer", commit, revisionLabel)
			if image.Ref != "playground-enforcer:"+commit || image.Want != commit {
				t.Errorf("inspected %q wanting %q", image.Ref, image.Want)
			}
			if image.Label != c.wantLabel || image.Missing != c.wantMissing {
				t.Errorf("label %q missing %q, want label %q missing %q", image.Label, image.Missing, c.wantLabel, c.wantMissing)
			}
			if image.Matches() {
				t.Errorf("%+v matches its pin", image)
			}
		})
	}
}

// labelledDaemon holds two images, each carrying both labels with only one of
// them set to its own pin, so reading the wrong label is a mismatch.
func labelledDaemon(_ context.Context, name string, args ...string) (string, error) {
	labels := map[string]map[string]string{
		"playground-enforcer:c0ffee": {"org.opencontainers.image.revision": "c0ffee", "org.opencontainers.image.version": "wrong"},
		"playground-verifier:0.26.1": {"org.opencontainers.image.revision": "wrong", "org.opencontainers.image.version": "0.26.1"},
	}
	if name != "docker" || len(args) != 5 {
		return "", fmt.Errorf("unexpected lookup %s %v", name, args)
	}
	for label, value := range labels[args[4]] {
		if strings.Contains(args[3], `"`+label+`"`) {
			return "sha256:" + value + " " + value, nil
		}
	}
	return "", errors.New("exit status 1: Error response from daemon: No such image: " + args[4])
}

func TestEachImageIsReadByItsOwnLabel(t *testing.T) {
	pins := []report.Pin{
		{Name: "ENFORCER_IMAGE", Value: "playground-enforcer"}, {Name: "ENFORCER_COMMIT", Value: "c0ffee"},
		{Name: "VERIFIER_IMAGE", Value: "playground-verifier"}, {Name: "VERIFIER_VERSION", Value: "0.26.1"},
	}
	found := images(context.Background(), labelledDaemon, pins)
	if len(found) != 2 {
		t.Fatalf("read %d images, want 2", len(found))
	}
	for _, image := range found {
		if !image.Matches() {
			t.Errorf("%+v does not match its pin", image)
		}
	}
}

func TestLabCommitCountsAnUntrackedFile(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), provenanceTimeout)
	defer cancel()
	git := func(args ...string) {
		base := []string{"-C", root, "-c", "user.name=lab", "-c", "user.email=lab@example.invalid", "-c", "commit.gpgsign=false"}
		if _, err := command(ctx, "git", append(base, args...)...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	git("init", "-q")
	writeFile(filepath.Join(root, "tracked"), "one\n")
	git("add", "tracked")
	git("commit", "-q", "-m", "one")
	if got := labCommit(ctx, command, root); strings.Contains(got, "uncommitted") || len(got) != 40 {
		t.Fatalf("a clean checkout reads as %q", got)
	}
	writeFile(filepath.Join(root, "scenarios", "new.yaml"), "id: new\n")
	if got := labCommit(ctx, command, root); !strings.Contains(got, "with uncommitted changes") {
		t.Errorf("a checkout with an untracked scenario reads as %q", got)
	}
}
