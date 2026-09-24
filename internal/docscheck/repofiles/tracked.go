package repofiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Tracked returns the files git tracks under root that are on disk: what a
// clone of the repository holds, which is what a reference from one file to
// another must name. Outside a git work tree, as in an export, the lister's
// find already lists the committed tree, so its answer is returned.
func Tracked(ctx context.Context, root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	if !inWorkTree(ctx, root) {
		return List(ctx, root)
	}
	command := exec.CommandContext(ctx, "git", "ls-files", "--cached", "-z")
	command.Dir = root
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return present(root, out)
}

// inWorkTree asks git the question the lister asks, so both lists take the
// same branch.
func inWorkTree(ctx context.Context, root string) bool {
	command := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	command.Dir = root
	out, err := command.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// present reads NUL-separated paths and keeps those still on disk, as the
// lister does; an empty result is refused like an empty list.
func present(root string, out []byte) ([]string, error) {
	var files []string
	for _, p := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if p == "" {
			continue
		}
		if err := checkLine(p); err != nil {
			return nil, err
		}
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		switch {
		case err == nil:
			files = append(files, p)
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: git tracks no file under %s", ErrInvalid, root)
	}
	return files, nil
}
