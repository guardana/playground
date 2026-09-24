package docsconfig

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/docscheck/glob"
)

func validateTypes(c Config) error {
	if len(c.Types) == 0 {
		return errors.New("types is empty")
	}
	for i, t := range c.Types {
		switch {
		case t.Name == "" || t.Heading == "":
			return fmt.Errorf("types[%d]: a type needs a name and a heading", i)
		case slices.ContainsFunc(c.Types[:i], func(u Type) bool { return u.Name == t.Name }):
			return fmt.Errorf("types: %s is listed twice", t.Name)
		case t.Exempt && t.Budget != 0:
			return fmt.Errorf("types: %s has a budget and is exempt", t.Name)
		case !t.Exempt && t.Budget <= 0:
			return fmt.Errorf("types: %s has no positive budget and is not exempt", t.Name)
		}
	}
	return nil
}

func validateAudiences(c Config) error {
	if len(c.Audiences) == 0 {
		return errors.New("audiences is empty")
	}
	for i, a := range c.Audiences {
		if a == "" || slices.Contains(c.Audiences[:i], a) {
			return fmt.Errorf("audiences: %q is empty or listed twice", a)
		}
	}
	return nil
}

func validateBudgets(c Config) error {
	if c.Readme.Root <= 0 || c.Readme.Folder <= 0 {
		return errors.New("readme.root and readme.folder must be positive")
	}
	for p, n := range c.Ceilings {
		if err := checkPath(p); err != nil {
			return fmt.Errorf("ceilings: %w", err)
		}
		if n <= 0 {
			return fmt.Errorf("ceilings: %s has no positive count", p)
		}
	}
	return nil
}

func validatePlaces(c Config) error {
	for _, check := range []func(Config) error{validateDirectories, validatePageTypes, validateExcluded} {
		if err := check(c); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectories(c Config) error {
	if len(c.Directories) == 0 {
		return errors.New("directories is empty")
	}
	for dir, t := range c.Directories {
		if err := checkPath(dir); err != nil || (dir != "docs" && !strings.HasPrefix(dir, "docs/")) {
			return fmt.Errorf("directories: %q is not a directory under docs", dir)
		}
		if _, ok := c.Lookup(t); !ok {
			return fmt.Errorf("directories: %s holds type %q, which types does not declare", dir, t)
		}
	}
	return nil
}

func validatePageTypes(c Config) error {
	for page, t := range c.PageTypes {
		if err := checkPath(page); err != nil || !strings.HasSuffix(page, ".md") {
			return fmt.Errorf("page_types: %q is not a page", page)
		}
		if _, ok := c.Lookup(t); !ok {
			return fmt.Errorf("page_types: %s has type %q, which types does not declare", page, t)
		}
	}
	return nil
}

func validateExcluded(c Config) error {
	for i, p := range c.Excluded {
		if err := checkPath(strings.TrimSuffix(p, "/")); err != nil {
			return fmt.Errorf("excluded: %w", err)
		}
		if slices.Contains(c.Excluded[:i], p) {
			return fmt.Errorf("excluded: %s is listed twice", p)
		}
	}
	return nil
}

func validateGlobs(c Config) error {
	if len(c.Surfaces) == 0 {
		return errors.New("surfaces is empty, so no change would ever need a page")
	}
	for name, list := range map[string][]string{"frozen": c.Frozen, "surfaces": c.Surfaces} {
		for i, p := range list {
			if _, err := glob.Compile(p); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			if slices.Contains(list[:i], p) {
				return fmt.Errorf("%s: %s is listed twice", name, p)
			}
		}
	}
	return nil
}

// checkPath admits a clean, relative slash path with no parent reference.
func checkPath(p string) error {
	switch {
	case p == "":
		return errors.New("a path is empty")
	case strings.HasPrefix(p, "/"), strings.Contains(p, "\\"):
		return fmt.Errorf("%q is not a relative slash path", p)
	case path.Clean(p) != p, p == "." || p == "..", strings.HasPrefix(p, "../"):
		return fmt.Errorf("%q is not a clean path", p)
	}
	return nil
}
