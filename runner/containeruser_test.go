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
)

// daemon answers the user probe as a docker daemon would: writer, and only
// writer ("*" for any user), gets a key written on the bind mount; info says
// whether it is rootless; fail, when set, is what every docker run answers.
type daemon struct {
	writer   string
	rootless bool
	fail     error
	calls    [][]string
}

func (d *daemon) run(_ context.Context, name string, args ...string) (string, error) {
	d.calls = append(d.calls, append([]string{name}, args...))
	if len(args) > 0 && args[0] == "info" {
		if d.rootless {
			return `["name=seccomp,profile=builtin","name=rootless"]`, nil
		}
		return `["name=seccomp,profile=builtin"]`, nil
	}
	if d.fail != nil {
		return "", d.fail
	}
	if !slices.Contains(args, "keygen") {
		return "", nil
	}
	if user := args[slices.Index(args, "--user")+1]; d.writer != "*" && user != d.writer {
		return "", errors.New("policy keygen: mkdir /probe/key: permission denied")
	}
	return "", writeProbeKey(args)
}

func (d *daemon) probedAs() []string {
	var users []string
	for _, call := range d.calls {
		if slices.Contains(call, "keygen") {
			users = append(users, call[slices.Index(call, "--user")+1])
		}
	}
	return users
}

// writeProbeKey writes the key the user probe's keygen asks for, as the
// runner's own user, into the directory its arguments mount at /probe.
func writeProbeKey(args []string) error {
	mount := args[slices.Index(args, "-v")+1]
	key := filepath.Join(strings.TrimSuffix(mount, ":/probe"), "key")
	if err := os.MkdirAll(key, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(key, "signing.key"), []byte("probe"), 0o600)
}

func ownIDs() string { return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()) }

func signedAs(t *testing.T, calls [][]string) string {
	t.Helper()
	for _, call := range calls {
		if slices.Contains(call, "sign") {
			return call[slices.Index(call, "--user")+1]
		}
	}
	t.Fatalf("nothing was signed: %v", calls)
	return ""
}

// The runner's own ids where the daemon keeps them, root under rootless
// Docker; the probe that succeeded is not run again.
func TestTheSignerRunsAsTheUserWhoseFilesTheRunnerOwns(t *testing.T) {
	for name, d := range map[string]*daemon{
		"a daemon that keeps the runner's ids": {writer: ownIDs()},
		"rootless Docker":                      {writer: "0:0", rootless: true},
	} {
		t.Run(name, func(t *testing.T) {
			sign := signWithImage("lab-enforcer:pin", d.run)
			for range 2 {
				if err := sign(context.Background(), "/keys", "/repo/config/policies/p.json", t.TempDir()); err != nil {
					t.Fatal(err)
				}
			}
			if got := signedAs(t, d.calls); got != d.writer {
				t.Errorf("signed as %s, want %s", got, d.writer)
			}
			want := []string{ownIDs()}
			if d.rootless {
				want = append(want, "0:0")
			}
			if got := d.probedAs(); !slices.Equal(got, want) {
				t.Errorf("probed as %v for two signatures, want %v", got, want)
			}
		})
	}
}

// Root is never tried on a daemon that is not rootless, whatever failed.
func TestTheProbeNeverTriesRootOnARootfulDaemon(t *testing.T) {
	d := &daemon{writer: "0:0"}
	if _, err := containerUser(context.Background(), "lab-enforcer:pin", d.run, fileOwner); err == nil {
		t.Fatal("a rootful daemon where only root writes was accepted")
	}
	if got := d.probedAs(); !slices.Equal(got, []string{ownIDs()}) {
		t.Errorf("probed as %v on a rootful daemon, want the runner's ids alone", got)
	}
}

// A key written and owned by another uid is a mapping the lab does not know;
// a docker that failed to run is reported as docker's failure.
func TestTheProbeSaysWhichFailed(t *testing.T) {
	other := func(string) (int, error) { return os.Getuid() + 1, nil }
	for name, c := range map[string]struct {
		d       *daemon
		ownerOf func(string) (int, error)
		says    string
	}{
		"files owned by another uid": {&daemon{writer: "*", rootless: true}, other, "maps users in a way the lab does not know"},
		"a docker that cannot run":   {&daemon{fail: errors.New("Mounts denied: /probe is not shared")}, fileOwner, "Mounts denied"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := containerUser(context.Background(), "lab-enforcer:pin", c.d.run, c.ownerOf)
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v, want an error saying %q", err, c.says)
			}
		})
	}
}

// A probe that failed, here on a cancelled context, is tried again by the
// next signature instead of failing every later scenario of the run.
func TestAFailedProbeIsTriedAgain(t *testing.T) {
	d := &daemon{writer: ownIDs()}
	run := func(ctx context.Context, name string, args ...string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return d.run(ctx, name, args...)
	}
	sign := signWithImage("lab-enforcer:pin", run)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sign(cancelled, "/keys", "/repo/config/policies/p.json", t.TempDir()); err == nil {
		t.Fatal("signed on a cancelled context")
	}
	if err := sign(context.Background(), "/keys", "/repo/config/policies/p.json", t.TempDir()); err != nil {
		t.Fatalf("the probe was not tried again: %v", err)
	}
	if got := signedAs(t, d.calls); got != ownIDs() {
		t.Errorf("signed as %s after the failed probe, want %s from a probe run again", got, ownIDs())
	}
	if got := d.probedAs(); !slices.Equal(got, []string{ownIDs()}) {
		t.Errorf("probed as %v, want the second call's probe alone", got)
	}
}
