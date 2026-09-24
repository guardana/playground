// Package frontmatter reads and writes the metadata block at the head of a
// documentation page, a strict subset of YAML with six known keys, and counts
// the words of the body that follows it.
//
// The grammar admits one spelling of each block, so Render is the inverse of
// Parse: a generator writes what the check reads, and a hand-written page
// either parses or names the line that stops it. Which types and audiences
// exist is data in docs/docs.json, judged by the page check, not here.
package frontmatter

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Meta is the metadata a page declares. The zero value is not a valid block.
type Meta struct {
	Title   string
	Summary string
	Type    string
	// Audience names who the page is written for, from the configuration's
	// audiences.
	Audience []string
	// Covers lists the files whose change makes the page suspect, as globs
	// where ** means any depth.
	Covers []string
	// Generated names the scripts/gen-*.go program that renders the page.
	Generated string
}

const (
	keyTitle     = "title"
	keySummary   = "summary"
	keyType      = "type"
	keyAudience  = "audience"
	keyCovers    = "covers"
	keyGenerated = "generated"
)

// keys is the one order a block spells them in.
var keys = []string{keyTitle, keySummary, keyType, keyAudience, keyCovers, keyGenerated}

const (
	// MaxSummary is the longest summary, in characters.
	MaxSummary = 160
	// MaxCovers is the most globs one page may cover.
	MaxCovers = 32

	generatedPrefix = "scripts/gen-"
	generatedSuffix = ".go"
)

// ErrInvalid is wrapped by every refusal of a block or a Meta.
var ErrInvalid = errors.New("frontmatter")

// Validate refuses a Meta no page may carry: a missing key, a summary past
// MaxSummary, an empty or repeated list item, too many covers, a generated
// value that names no generator, and any value the block could not spell.
func Validate(m Meta) error {
	for _, check := range []func(Meta) error{checkScalars, checkRequired, checkLists, checkGenerated} {
		if err := check(m); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	return nil
}

func checkScalars(m Meta) error {
	for _, kv := range []struct{ key, value string }{
		{keyTitle, m.Title}, {keySummary, m.Summary}, {keyType, m.Type}, {keyGenerated, m.Generated},
	} {
		if kv.value == "" {
			continue
		}
		if err := checkScalar(kv.value); err != nil {
			return fmt.Errorf("%s: %w", kv.key, err)
		}
	}
	return nil
}

func checkRequired(m Meta) error {
	switch {
	case m.Title == "":
		return errors.New("title is required")
	case m.Summary == "":
		return errors.New("summary is required")
	case utf8.RuneCountInString(m.Summary) > MaxSummary:
		return fmt.Errorf("summary is %d characters, the most is %d", utf8.RuneCountInString(m.Summary), MaxSummary)
	case m.Type == "":
		return errors.New("type is required")
	case len(m.Audience) == 0:
		return errors.New("audience is required")
	case len(m.Covers) == 0:
		return errors.New("covers is required")
	case len(m.Covers) > MaxCovers:
		return fmt.Errorf("covers lists %d globs, the most is %d", len(m.Covers), MaxCovers)
	}
	return nil
}

func checkLists(m Meta) error {
	for _, list := range []struct {
		key   string
		items []string
	}{{keyAudience, m.Audience}, {keyCovers, m.Covers}} {
		for i, item := range list.items {
			if err := checkItem(item); err != nil {
				return fmt.Errorf("%s[%d]: %w", list.key, i, err)
			}
			if slices.Contains(list.items[:i], item) {
				return fmt.Errorf("%s lists %q twice", list.key, item)
			}
		}
	}
	return nil
}

func checkGenerated(m Meta) error {
	if m.Generated == "" {
		return nil
	}
	name, ok := strings.CutPrefix(m.Generated, generatedPrefix)
	if !ok || !strings.HasSuffix(name, generatedSuffix) || len(name) <= len(generatedSuffix) || strings.Contains(name, "/") {
		return fmt.Errorf("generated %q does not name a %s<name>%s program", m.Generated, generatedPrefix, generatedSuffix)
	}
	return nil
}
