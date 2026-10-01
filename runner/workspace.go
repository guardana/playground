package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/guardana/playground/internal/labspec"
)

// workspaceVariable names a directory outside the clone, laid out like the
// lab, that a run reads its scenarios and every file they name from. Compose
// mounts the workspace's directories under the same name.
const workspaceVariable = "LAB_WORKSPACE"

// workspace is where a run's scenarios and the files they name are read from:
// the clone itself, or the directory LAB_WORKSPACE names. The lab's own files
// (versions.env, the classification, fingerprints, compose) stay the clone's.
type workspace struct {
	// dir is absolute with every link resolved.
	dir      string
	external bool
}

// openWorkspace reads LAB_WORKSPACE and refuses one the run could not keep
// apart from the lab: inside the clone its files would mix with the lab's,
// inside the reports they would sit among what services write.
func openWorkspace(root, reports string, environ []string) (workspace, error) {
	value, set := lookupSet(environ, workspaceVariable)
	if !set {
		if err := refuseReportsInMounts(resolved(root), reports); err != nil {
			return workspace{}, err
		}
		return workspace{dir: resolved(root)}, refuseReportsInClone(root, reports)
	}
	if err := refuseReportsInClone(root, reports); err != nil {
		return workspace{}, err
	}
	if strings.TrimSpace(value) == "" {
		return workspace{}, fmt.Errorf("%s is set and empty; name a directory, or unset it to run the lab's own scenarios",
			workspaceVariable)
	}
	dir := resolved(value)
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return workspace{}, fmt.Errorf("%s: %w", workspaceVariable, err)
	case !info.IsDir():
		return workspace{}, fmt.Errorf("%s %s is not a directory", workspaceVariable, value)
	case within(resolved(reports), dir):
		return workspace{}, fmt.Errorf("the reports directory %s is inside the workspace %s; containers mount workspace directories, so keep the reports outside it",
			reports, value)
	case within(dir, resolved(root)):
		return workspace{}, fmt.Errorf("%s %s is inside the clone %s; keep a workspace outside it", workspaceVariable, value, root)
	case within(dir, resolved(reports)):
		return workspace{}, fmt.Errorf("%s %s is inside the reports directory %s; keep a workspace outside it",
			workspaceVariable, value, reports)
	}
	return workspace{dir: dir, external: true}, nil
}

// refuseReportsInMounts refuses reports inside a directory of the clone that a
// container mounts: every run's gateway/, pki/ and journals would be in the
// agent's or a double's view.
func refuseReportsInMounts(root, reports string) error {
	for _, dir := range mountedDirs() {
		if within(resolved(reports), filepath.Join(root, filepath.FromSlash(strings.Trim(dir, "/")))) {
			return fmt.Errorf("the reports directory %s is inside %s, which containers mount; keep the reports under the clone's reports/ or outside the clone",
				reports, dir)
		}
	}
	return nil
}

// refuseReportsInClone takes reports inside the clone only at or under its
// reports/, the one directory there that no service mounts and the build
// context leaves out: anywhere else a run's records, and the collector's key
// written before the images are built, would sit in a container's view or in
// an image.
func refuseReportsInClone(root, reports string) error {
	clone, at := resolved(root), resolved(reports)
	written, err := filepath.Abs(reports)
	if err != nil {
		return err
	}
	lexical, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if !within(at, clone) && !within(written, lexical) {
		return nil
	}
	own := filepath.Join(clone, "reports")
	info, err := os.Lstat(own)
	switch {
	case err == nil && info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("the clone's reports/ at %s is a link; make it a directory, or keep the reports outside the clone", own)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return err
	case !within(at, own):
		return fmt.Errorf("the reports directory %s is inside the clone %s and not under its reports/, the only directory there no container mounts and no image is built from",
			reports, root)
	}
	return nil
}

// file is a path a scenario names, in the workspace.
func (w workspace) file(named string) string { return filepath.Join(w.dir, filepath.FromSlash(named)) }

// namedFile is one file a scenario names and the directory it has to resolve
// into: the one a container mounts it from, or the runner reads it from.
type namedFile struct{ field, path, dir string }

func namedFiles(spec labspec.Scenario) []namedFile {
	files := []namedFile{
		{"trajectory", spec.Trajectory, trajectoriesDir},
	}
	if plan := spec.Gateway; plan != nil {
		files = append(files,
			namedFile{"gateway.config", plan.Config, labspec.GatewayConfigDir},
			namedFile{"gateway.policy", plan.Policy, labspec.PolicyDir},
			scriptFile("gateway.pdp_script", plan.PDPScript, "config/pdp/"),
			scriptFile("gateway.approver_script", plan.ApproverScript, "config/approver/"))
	}
	if spec.Trace != nil {
		files = append(files, namedFile{"trace.contract", spec.Trace.Contract, labspec.ContractDir})
	}
	return files
}

// scriptFile is a double's script, which a scenario names by its file name.
func scriptFile(field, name, dir string) namedFile {
	if name == "" {
		return namedFile{field: field, dir: dir}
	}
	return namedFile{field, path.Join(dir, name), dir}
}

// refuseMissing refuses a scenario naming a file the workspace does not hold,
// before anything boots: a missing script surfaces otherwise as a double that
// exits, far from its cause.
func (w workspace) refuseMissing(spec labspec.Scenario) error {
	if w.external {
		if err := w.refuseLinkedDirs(); err != nil {
			return err
		}
	}
	for _, named := range namedFiles(spec) {
		if named.path == "" {
			continue
		}
		if err := w.holds(named); err != nil {
			return err
		}
	}
	return nil
}

func (w workspace) holds(named namedFile) error {
	found, err := filepath.EvalSymlinks(w.file(named.path))
	if err != nil {
		return fmt.Errorf("%s %s is not in the workspace %s: %w", named.field, named.path, w.dir, err)
	}
	info, err := os.Stat(found)
	switch {
	case err != nil:
		return fmt.Errorf("%s %s: %w", named.field, named.path, err)
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s %s is not a regular file", named.field, named.path)
	case !within(found, resolved(w.file(named.dir))):
		return fmt.Errorf("%s %s resolves to %s, outside %s, which is all a container or the runner reads it from",
			named.field, named.path, found, named.dir)
	}
	return nil
}

// mountedDirs are the workspace directories a container or the signer binds
// by the path as written, so Docker follows a link anywhere along it.
func mountedDirs() []string {
	return []string{
		"trajectories", labspec.ContractDir, "config/pdp", "config/approver", labspec.PolicyDir, labspec.GatewayConfigDir,
	}
}

// refuseLinkedDirs refuses a workspace where any directory on the way to one
// that is mounted is a link: a container would see what the link points at,
// which the runner's own checks of the files it names never looked at.
func (w workspace) refuseLinkedDirs() error {
	for _, dir := range mountedDirs() {
		at := w.dir
		for _, part := range strings.Split(strings.Trim(dir, "/"), "/") {
			at = filepath.Join(at, part)
			info, err := os.Lstat(at)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			switch {
			case err != nil:
				return fmt.Errorf("%s: %w", at, err)
			case info.Mode()&fs.ModeSymlink != 0:
				return fmt.Errorf("%s in the workspace is a link; a container would see what it points at, so make it a directory",
					strings.TrimPrefix(at, w.dir+string(filepath.Separator)))
			}
		}
	}
	return nil
}

// lookupSet reads one variable and says whether it is set at all, which an
// empty value alone cannot. The last entry wins, as it does for a child process.
func lookupSet(environ []string, name string) (string, bool) {
	value, set := "", false
	for _, entry := range environ {
		if key, rest, ok := strings.Cut(entry, "="); ok && key == name {
			value, set = rest, true
		}
	}
	return value, set
}
