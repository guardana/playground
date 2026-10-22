package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/guardana/playground/internal/labspec"
	"github.com/guardana/playground/runner/check"
)

const (
	// scanService is the verifier with no network, which a scan step runs in
	// with its working directory at the step's inputs.
	scanService = "scan-verifier"
	// scanTarget is the directory under the step's in/ a target is copied to,
	// and so the name the verifier gives every file it scans.
	scanTarget = "target"
	// maxScanFiles and maxScanDepth bound a target the runner copies.
	maxScanFiles = 256
	maxScanDepth = 8
)

func newScanRun(number int, dir string) check.VerifierRun {
	return check.VerifierRun{Step: number, URL: scanTarget, Report: filepath.Join(dir, fmt.Sprintf("step-%d.json", number))}
}

// runScan copies a scan step's target into its inputs and scans it. A target
// that cannot be copied whole is not scanned: a scan of part of it would be
// graded as a scan of all of it.
func (l lab) runScan(
	ctx context.Context, compose Compose, profiles []string, dir string, made error, scan labspec.ScanStep, run *check.VerifierRun,
) {
	if made != nil {
		run.Detail = "the verifier's directory could not be made: " + made.Error()
		return
	}
	if err := handOverScan(l.workspace, dir, run.Step, scan); err != nil {
		run.Detail = "the step's inputs could not be handed over: " + err.Error()
		return
	}
	args := []string{"scan", scanTarget, "--format", "json", "--plugins", "builtins"}
	if scan.Profile != "" {
		args = append(args, "--profile", path.Join(verifierMount, inputsDir, profileInput))
	}
	step := compose.WithEnv(map[string]string{stepVariable: stepDirName(run.Step)})
	l.runOnce(ctx, step, profiles, scanService, args, run)
}

// handOverScan makes the step's directories, copies the target into in/target
// and the profile beside it.
func handOverScan(space workspace, dir string, number int, scan labspec.ScanStep) error {
	if err := makeStepDirs(dir, number, false); err != nil {
		return err
	}
	in := filepath.Join(dir, stepDirName(number), inputsDir)
	if scan.Profile != "" {
		if err := copyInput(space.file(scan.Profile), filepath.Join(in, profileInput)); err != nil {
			return fmt.Errorf("the profile %s: %w", scan.Profile, err)
		}
	}
	if err := refuseLinkOnTheWay(space.dir, scan.Target); err != nil {
		return fmt.Errorf("the target %s: %w", scan.Target, err)
	}
	if err := copyTree(space.file(scan.Target), filepath.Join(in, scanTarget)); err != nil {
		return fmt.Errorf("the target %s: %w", scan.Target, err)
	}
	return nil
}

// copyTree copies a directory of regular files through an os.Root, so a link
// in it is refused rather than followed. A name the verifier skips is refused
// too: a file it did not read would be absent from its scope without the
// scenario saying so.
func copyTree(from, to string) error {
	root, err := os.OpenRoot(from)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := makeDir(to, inputDirMode); err != nil {
		return err
	}
	files := 0
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case name == ".":
			return nil
		case skippedByVerifier(entry):
			return fmt.Errorf("%s is hidden or a directory the verifier skips", name)
		case strings.Count(name, "/") >= maxScanDepth:
			return fmt.Errorf("%s is deeper than %d", name, maxScanDepth)
		case entry.IsDir():
			return makeDir(filepath.Join(to, filepath.FromSlash(name)), inputDirMode)
		case !entry.Type().IsRegular():
			return fmt.Errorf("%s is not a regular file", name)
		}
		if files++; files > maxScanFiles {
			return errors.New("the target holds more files than the bound")
		}
		return copyFromRoot(root, name, filepath.Join(to, filepath.FromSlash(name)))
	})
}

// skippedByVerifier is a hidden name, or a directory every scan skips
// wherever it occurs (the verifier's usage-scan.md at the pin).
func skippedByVerifier(entry fs.DirEntry) bool {
	name := entry.Name()
	if strings.HasPrefix(name, ".") {
		return true
	}
	if !entry.IsDir() {
		return false
	}
	switch name {
	case "venv", "env", "__pycache__", "node_modules", "dist", "build":
		return true
	}
	return strings.HasSuffix(name, ".egg-info")
}

// copyFromRoot reads one file through the root, never following a link and
// never blocking on a pipe, and only a regular file within the bound.
func copyFromRoot(root *os.Root, name, to string) error {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	switch {
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", name)
	case info.Size() > maxInputBytes:
		return fmt.Errorf("%s is larger than %d bytes", name, maxInputBytes)
	}
	body, err := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxInputBytes {
		return fmt.Errorf("%s grew past %d bytes", name, maxInputBytes)
	}
	return keepNew(to, string(body), inputMode)
}

// refuseLinkOnTheWay refuses a target any directory on the way to which, the
// target included, is a link: os.OpenRoot follows one in the path it opens.
func refuseLinkOnTheWay(workspace, target string) error {
	at := workspace
	for _, part := range strings.Split(strings.Trim(target, "/"), "/") {
		at = filepath.Join(at, part)
		info, err := os.Lstat(at)
		switch {
		case err != nil:
			return err
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%s is a link", at)
		}
	}
	return nil
}
