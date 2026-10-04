package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// errNotTheRunners is a probe whose file was written and belongs to another uid.
var errNotTheRunners = errors.New("the file it wrote belongs to another uid")

// containerUser picks the --user under which what a container writes on a
// bind mount belongs to whoever runs the runner: its own ids where the daemon
// keeps them, root under rootless Docker, where the container's root is that
// user. Root is never tried on a daemon that does not say it is rootless, and
// a mapping under which neither holds is refused rather than guessed, since
// the enforcer refuses a key another account owns.
func containerUser(ctx context.Context, image string, run lookup, ownerOf func(string) (int, error)) (string, error) {
	own := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	ownErr := writesAsRunner(ctx, image, own, run, ownerOf)
	if ownErr == nil {
		return own, nil
	}
	info, err := run(ctx, "docker", "info", "--format", "{{json .SecurityOptions}}")
	if err != nil {
		return "", fmt.Errorf("the user probe as %s: %w; and docker info: %w", own, ownErr, err)
	}
	if !strings.Contains(info, "name=rootless") {
		return "", fmt.Errorf("the user probe as %s: %w", own, ownErr)
	}
	rootErr := writesAsRunner(ctx, image, "0:0", run, ownerOf)
	if rootErr == nil {
		return "0:0", nil
	}
	if errors.Is(ownErr, errNotTheRunners) && errors.Is(rootErr, errNotTheRunners) {
		return "", fmt.Errorf("under rootless Docker neither %s nor 0:0 writes files this user owns; "+
			"the daemon maps users in a way the lab does not know", own)
	}
	return "", fmt.Errorf("the user probe as %s: %w; as 0:0: %w", own, ownErr, rootErr)
}

// writesAsRunner has the enforcer's keygen write a throwaway key as user into
// a directory of the runner's, and says why the result is not the runner's.
func writesAsRunner(ctx context.Context, image, user string, run lookup, ownerOf func(string) (int, error)) error {
	probe, err := os.MkdirTemp("", "lab-user-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(probe) }()
	if _, err := run(ctx, "docker", "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", user,
		"-v", probe+":/probe", "--entrypoint", "/enforcer/control", image,
		"policy", "keygen", "--out", "/probe/key"); err != nil {
		return err
	}
	owner, err := ownerOf(filepath.Join(probe, "key", "signing.key"))
	if err != nil {
		return err
	}
	if owner != os.Getuid() {
		return errNotTheRunners
	}
	return nil
}

// fileOwner is the uid that owns path, read without following a link.
func fileOwner(path string) (int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file this system can name an owner for", path)
	}
	return int(stat.Uid), nil
}

// onceUser runs containerUser until it succeeds and answers every later call
// from that success; a failed probe, a cancelled one included, is tried again
// by the next call rather than kept for the rest of the process.
func onceUser(image string, run lookup) func(context.Context) (string, error) {
	var mu sync.Mutex
	var user string
	return func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if user != "" {
			return user, nil
		}
		found, err := containerUser(ctx, image, run, fileOwner)
		if err != nil {
			return "", err
		}
		user = found
		return user, nil
	}
}
