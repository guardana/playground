package impact

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
)

// ParseNameOnly reads the output of `git diff --name-only`: one clean path
// per line, none twice. A path git quoted holds a character the list would
// refuse too, so the line is refused rather than unquoted.
func ParseNameOnly(out []byte) ([]string, error) {
	if len(out) == 0 {
		return nil, nil
	}
	if strings.Contains(string(out), "\r") {
		return nil, fmt.Errorf("%w: git's output holds a carriage return", ErrInvalid)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for i, line := range lines {
		if err := checkGitPath(line); err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", ErrInvalid, i+1, err)
		}
		if slices.Contains(paths, line) {
			return nil, fmt.Errorf("%w: line %d: %s is listed twice", ErrInvalid, i+1, line)
		}
		paths = append(paths, line)
	}
	return paths, nil
}

func checkGitPath(line string) error {
	if strings.HasPrefix(line, "\"") {
		return fmt.Errorf("git quoted the path %s; it holds a character the list refuses", line)
	}
	return checkPath(line)
}

// checkPath admits a clean, relative slash path with no parent reference and
// no white space, control byte or byte order mark: what git and the
// repository's file list produce.
func checkPath(p string) error {
	switch {
	case p == "":
		return errors.New("a path is empty")
	case strings.Contains(p, "\ufeff"):
		return fmt.Errorf("%q holds a byte order mark", p)
	case strings.ContainsFunc(p, unicode.IsSpace):
		return fmt.Errorf("%q holds white space", p)
	case strings.ContainsFunc(p, unicode.IsControl):
		return fmt.Errorf("%q holds a control byte", p)
	case strings.HasPrefix(p, "/"), strings.Contains(p, "\\"):
		return fmt.Errorf("%q is not a relative slash path", p)
	case path.Clean(p) != p, p == "." || p == "..", strings.HasPrefix(p, "../"):
		return fmt.Errorf("%q is not a clean path", p)
	}
	return nil
}
