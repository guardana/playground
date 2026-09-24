package impact

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Runner runs git with the arguments and returns its standard output. The
// program hands in one that executes the binary; a test hands in a table.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// ErrNotMeasured is wrapped by every result git could not produce: an absent
// git never reads as "no change" or "no commit".
var ErrNotMeasured = errors.New("NOT MEASURED")

func notMeasured(step string, err error) error {
	if errors.Is(err, ErrNotMeasured) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", ErrNotMeasured, step, err)
}

// gitRedirects point git at a repository other than the one found from its
// working directory; the Runner never passes them on.
var gitRedirects = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CEILING_DIRECTORIES"}

// Git returns the Runner that executes the binary in dir. Discovery stops one
// directory up, so an export placed inside another repository is not measured
// against that repository's log.
func Git(dir string, environ []string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- the arguments are this package's own tables.
		cmd.Dir = dir
		cmd.Env = append(slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
			name, _, _ := strings.Cut(kv, "=")
			return slices.Contains(gitRedirects, name)
		}), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
		out, err := cmd.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				reason, _, _ := strings.Cut(strings.TrimSpace(string(exit.Stderr)), "\n")
				return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, reason)
			}
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return out, nil
	}
}

// Repository binds a Runner to one working directory: before every call it
// asks git for its top level and refuses, as not measured, one whose real
// path is not the directory's own.
func Repository(run Runner, wd string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		out, err := run(ctx, "rev-parse", "--show-toplevel")
		if err != nil {
			return nil, notMeasured("git rev-parse", err)
		}
		top, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
		if err != nil {
			return nil, fmt.Errorf("%w: git's top level: %w", ErrNotMeasured, err)
		}
		here, err := filepath.EvalSymlinks(wd)
		if err != nil {
			return nil, fmt.Errorf("%w: the working directory: %w", ErrNotMeasured, err)
		}
		if top != here {
			return nil, fmt.Errorf("%w: git's top level %s is not the working directory %s", ErrNotMeasured, top, here)
		}
		return run(ctx, args...)
	}
}

// Changed asks git for the paths a range changed, a rename as both its old
// and its new path. A range spelled like an option is refused before git
// sees it.
func Changed(ctx context.Context, run Runner, rng string) ([]string, error) {
	if rng == "" || strings.HasPrefix(rng, "-") {
		return nil, fmt.Errorf("%w: %q is not a revision range", ErrInvalid, rng)
	}
	out, err := run(ctx, "diff", "--name-only", "--no-renames", rng, "--")
	if err != nil {
		return nil, notMeasured("git diff", err)
	}
	return ParseNameOnly(out)
}
