package control

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/guardana/playground/internal/hostpath"
	"github.com/guardana/playground/runner/driver"
)

// Signer signs a policy document into a bundle with the lab key. The key is
// mounted only into the one-shot container that signs, never into the gateway.
type Signer func(ctx context.Context, keysDir, policy, outDir string) error

const bundleName = "policy.bundle"

// signWithEnforcer signs with the pinned enforcer's own `policy sign`, in a
// container with no network, no capability and no image but the local pin, as
// a user whose files the invoking user owns, so the key's owner reads it.
func signWithEnforcer(root string, run driver.Lookup) Signer {
	var once sync.Once
	var sign Signer
	var err error
	return func(ctx context.Context, keysDir, policy, outDir string) error {
		once.Do(func() {
			var pins driver.Pins
			if pins, err = driver.ReadPins(filepath.Join(root, versionFile)); err == nil {
				sign = signWithImage(pins.Value("ENFORCER_IMAGE")+":"+pins.Value("ENFORCER_COMMIT"), run)
			}
		})
		if err != nil {
			return err
		}
		return sign(ctx, keysDir, policy, outDir)
	}
}

// signWithImage signs with the `policy sign` of the enforcer image named.
func signWithImage(image string, run driver.Lookup) Signer {
	user := onceUser(image, run)
	return func(ctx context.Context, keysDir, policy, outDir string) error {
		as, err := user(ctx)
		if err != nil {
			return err
		}
		_, err = run(ctx, "docker", "run", "--rm", "--pull", "never", "--network", "none", "--read-only",
			"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--user", as,
			"-v", keysDir+":/key:ro", "-v", filepath.Dir(policy)+":/policy:ro", "-v", outDir+":/out",
			"--entrypoint", "/enforcer/control", image,
			"policy", "sign", "--key", "/key/signing.key", "--out", "/out/"+bundleName, "/policy/"+filepath.Base(policy))
		return err
	}
}

// signInto signs in a directory of its own and copies the bundle alone into
// the run's gateway directory, which the enforcer mounts: whatever else the
// signing container leaves behind never reaches the plane.
func (d *Driver) signInto(ctx context.Context, policy, gatewayDir string) error {
	scratch, err := os.MkdirTemp("", "lab-sign-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	if err := d.sign(ctx, d.config.KeysDir, policy, scratch); err != nil {
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

// KeysDir is where scripts/lab-key.sh keeps the lab key on this machine.
func KeysDir(lookup func(string) string) string {
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
func (d *Driver) refuseKeysPlace() error {
	keys := hostpath.Resolved(d.config.KeysDir)
	places := map[string]string{"the clone": d.config.Root, "the reports directory": d.config.Reports}
	if d.config.Workspace.External {
		places["the workspace"] = d.config.Workspace.Dir
	}
	for name, dir := range places {
		if hostpath.Within(keys, hostpath.Resolved(dir)) {
			return fmt.Errorf("LAB_KEYS_DIR %s is inside %s; keep the lab key outside the clone, the reports and the workspace",
				d.config.KeysDir, name)
		}
	}
	return nil
}
