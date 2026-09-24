// Package docsconfig reads docs/docs.json: the page types and their word
// budgets, the audiences, the directory each type lives in, the README
// budgets, the pages pinned over budget, and the paths the impact report
// judges a change against.
package docsconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// Path is where the file lives, relative to the repository.
const Path = "docs/docs.json"

// Config is the parsed file. Every field is required unless its comment says
// it may be empty.
type Config struct {
	// Types is every page type, in the order the index lists them.
	Types     []Type   `json:"types"`
	Audiences []string `json:"audiences"`
	// Readme bounds README.md files outside docs/, which carry no frontmatter.
	Readme Readme `json:"readme"`
	// Ceilings pins a file over its budget to a word count it may not
	// exceed, so a rise is a visible edit here. May be empty.
	Ceilings map[string]int `json:"ceilings"`
	// Directories maps a directory under docs/ to the type its pages
	// declare; a page in no listed directory is refused.
	Directories map[string]string `json:"directories"`
	// PageTypes overrides the directory's type for one page. May be empty.
	PageTypes map[string]string `json:"page_types"`
	// Frozen names, as globs, the paths whose change is a contract change.
	// May be empty.
	Frozen []string `json:"frozen"`
	// Excluded names paths, and directories spelled with a trailing slash,
	// outside the documentation system. May be empty.
	Excluded []string `json:"excluded"`
	// Surfaces are the globs of code a page must cover.
	Surfaces []string `json:"surfaces"`
}

// Type is one kind of page. A type either has a positive word budget or is
// exempt from one, never both.
type Type struct {
	Name    string `json:"name"`
	Heading string `json:"heading"`
	Budget  int    `json:"budget"`
	Exempt  bool   `json:"exempt"`
}

// Readme is the word budget of README.md files.
type Readme struct {
	Root   int `json:"root"`
	Folder int `json:"folder"`
}

// ErrInvalid is wrapped by every refusal.
var ErrInvalid = errors.New("docs.json")

// Parse reads the file. It refuses an unknown or repeated key, a missing
// section and every value the checks could not act on.
func Parse(data []byte) (Config, error) {
	var c Config
	if err := checkKeysOnce(data); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: text after the document", ErrInvalid)
	}
	for _, check := range []func(Config) error{validateTypes, validateAudiences, validateBudgets, validatePlaces, validateGlobs} {
		if err := check(c); err != nil {
			return Config{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	return c, nil
}

// Lookup returns the declared type of that name.
func (c Config) Lookup(name string) (Type, bool) {
	i := slices.IndexFunc(c.Types, func(t Type) bool { return t.Name == name })
	if i < 0 {
		return Type{}, false
	}
	return c.Types[i], true
}

// TypeOf returns the type a page must declare: its own override, or its
// directory's. A page in no listed directory has none.
func (c Config) TypeOf(page string) (string, bool) {
	if t, ok := c.PageTypes[page]; ok {
		return t, true
	}
	t, ok := c.Directories[path.Dir(page)]
	return t, ok
}

// Excludes reports whether a path is excluded or lies in an excluded
// directory.
func (c Config) Excludes(rel string) bool {
	return slices.ContainsFunc(c.Excluded, func(entry string) bool {
		return rel == entry || strings.HasSuffix(entry, "/") && strings.HasPrefix(rel, entry)
	})
}
