package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The fixture is compiled in: the run stage of the image has no shell and no
// package manager, so a file the binary expects to find on disk is one more
// thing that can be absent and look like a defect in the lab.
//
//go:embed all:fixture
var fixture embed.FS

// presentedRoot is the prefix every tool path carries, and in the container it
// is also where the files live. A test points the sandbox at a temporary
// directory and the paths a scenario uses do not change.
const presentedRoot = "/data"

var errOutsideSandbox = errors.New("path escapes the sandbox")

// sandbox is the only thing this server does not lie about. Containment is
// enforced twice: the path is checked before it is used, and every operation
// goes through os.Root, which refuses a name that leaves the directory even by
// way of a symbolic link. A lab that can write outside its own directory is
// not a lab.
type sandbox struct {
	root *os.Root
}

func newSandbox(directory string) (*sandbox, error) {
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, err
	}
	// Seeded on every start, so a scenario that writes into the sandbox does
	// not change what the next run reads.
	if err := seed(directory); err != nil {
		return nil, err
	}
	opened, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &sandbox{root: opened}, nil
}

func seed(directory string) error {
	return fs.WalkDir(fixture, "fixture", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(directory, filepath.FromSlash(strings.TrimPrefix(name, "fixture")))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		body, err := fixture.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o600)
	})
}

func (s *sandbox) read(given string) (string, error) {
	name, err := locate(given)
	if err != nil {
		return "", err
	}
	body, err := s.root.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *sandbox) write(given, content string) error {
	name, err := locate(given)
	if err != nil {
		return err
	}
	if parent := path.Dir(name); parent != "." {
		if err := s.root.MkdirAll(parent, 0o750); err != nil {
			return err
		}
	}
	return s.root.WriteFile(name, []byte(content), 0o600)
}

func (s *sandbox) list(given string) ([]string, error) {
	name, err := locate(given)
	if err != nil {
		return nil, err
	}
	directory, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()

	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name()+"/")
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// locate maps a path a tool was given onto a name inside the sandbox. A path
// that does not start at the presented root, or that is not already clean, is
// refused here rather than repaired: repairing one is how a sandbox that
// refuses ../ ends up serving the file anyway.
func locate(given string) (string, error) {
	slashed := filepath.ToSlash(given)
	if slashed != presentedRoot && !strings.HasPrefix(slashed, presentedRoot+"/") {
		return "", fmt.Errorf("%w: %s", errOutsideSandbox, given)
	}
	name := strings.TrimPrefix(strings.TrimPrefix(slashed, presentedRoot), "/")
	if name == "" {
		name = "."
	}
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("%w: %s", errOutsideSandbox, given)
	}
	return name, nil
}
