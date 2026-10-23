package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// dockerSays answers `docker image inspect` the way the daemon would, with the
// output or error given for every image.
func dockerSays(out string, err error) Lookup {
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
			image := InspectImage(context.Background(), dockerSays(c.out, c.err), "playground-enforcer", commit, "org.opencontainers.image.revision")
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
