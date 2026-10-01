package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// signer signs a policy document into a bundle with the lab key. The key is
// mounted only into the one-shot container that signs, never into the gateway.
type signer func(ctx context.Context, keysDir, policy, outDir string) error

const bundleName = "policy.bundle"

// signWithEnforcer signs with the pinned enforcer's own `policy sign`, in a
// container with no network, no capability and no image but the local pin, as
// the invoking user so the key's owner reads it.
func signWithEnforcer(root string, run lookup) signer {
	return func(ctx context.Context, keysDir, policy, outDir string) error {
		pins, err := readPins(filepath.Join(root, versionFile))
		if err != nil {
			return err
		}
		image := pinValue(pins, "ENFORCER_IMAGE") + ":" + pinValue(pins, "ENFORCER_COMMIT")
		return signWithImage(image, run)(ctx, keysDir, policy, outDir)
	}
}

// signWithImage signs with the `policy sign` of the enforcer image named.
func signWithImage(image string, run lookup) signer {
	return func(ctx context.Context, keysDir, policy, outDir string) error {
		_, err := run(ctx, "docker", "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
			"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
			"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			"-v", keysDir+":/key:ro", "-v", filepath.Dir(policy)+":/policy:ro", "-v", outDir+":/out",
			"--entrypoint", "/enforcer/control", image,
			"policy", "sign", "--key", "/key/signing.key", "--out", "/out/"+bundleName, "/policy/"+filepath.Base(policy))
		return err
	}
}

// signInto signs in a directory of its own and copies the bundle alone into
// the run's gateway directory, which the enforcer mounts: whatever else the
// signing container leaves behind never reaches the plane.
func (l lab) signInto(ctx context.Context, policy, gatewayDir string) error {
	scratch, err := os.MkdirTemp("", "lab-sign-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := l.sign(ctx, l.keysDir, policy, scratch); err != nil {
		return err
	}
	return copyRegular(filepath.Join(scratch, bundleName), filepath.Join(gatewayDir, bundleName))
}

// copyRegular copies one regular file, refusing a link or anything else the
// signer might have left under the bundle's name.
func copyRegular(from, to string) error {
	info, err := os.Lstat(from)
	switch {
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", from)
	}
	source, err := os.Open(from) // #nosec G304 -- the bundle the signer just wrote.
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G302,G304,G703 -- public, inside the run directory.
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	return target.Close()
}

// labKeysDir is where scripts/lab-key.sh keeps the lab key on this machine.
func labKeysDir(lookup func(string) string) string {
	if dir := lookup("LAB_KEYS_DIR"); dir != "" {
		return dir
	}
	state := lookup("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(lookup("HOME"), ".local", "state")
	}
	return filepath.Join(state, "guardana-playground", "lab-key")
}

// refuseKeysPlace refuses a key directory inside the clone, where it could be
// committed, inside the reports, which services write into and people share,
// or inside a workspace, whose directories containers mount.
func (l lab) refuseKeysPlace() error {
	keys := resolved(l.keysDir)
	places := map[string]string{"the clone": l.root, "the reports directory": l.reports}
	if l.workspace.external {
		places["the workspace"] = l.workspace.dir
	}
	for name, dir := range places {
		if within(keys, resolved(dir)) {
			return fmt.Errorf("LAB_KEYS_DIR %s is inside %s; keep the lab key outside the clone, the reports and the workspace",
				l.keysDir, name)
		}
	}
	return nil
}

// resolved is path made absolute with every link resolved. A path that does
// not exist yet keeps its missing tail under its nearest existing directory,
// resolved, so it compares with paths that do exist.
func resolved(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if evaluated, err := filepath.EvalSymlinks(absolute); err == nil {
		return evaluated
	}
	parent := filepath.Dir(absolute)
	if parent == absolute {
		return absolute
	}
	return filepath.Join(resolved(parent), filepath.Base(absolute))
}

// within reports whether path is dir or lies below it. Spellings are compared
// first; then every existing ancestor of path is compared with dir as a file,
// because a case-insensitive file system or a second name for a volume gives
// one directory several spellings that resolving links does not reconcile.
func within(path, dir string) bool {
	relative, err := filepath.Rel(dir, path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return true
	}
	target, err := os.Stat(dir)
	if err != nil {
		return false
	}
	for at := filepath.Clean(path); ; at = filepath.Dir(at) {
		if info, err := os.Stat(at); err == nil && os.SameFile(info, target) {
			return true
		}
		if filepath.Dir(at) == at {
			return false
		}
	}
}
