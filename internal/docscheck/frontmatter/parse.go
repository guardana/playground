package frontmatter

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

const fence = "---"

// Parse reads the block at the head of a page and returns its Meta and the
// body after the closing fence. The page starts with a fence line, and every
// line up to the next fence is one known key, in the declared order, each
// once, followed by a colon, a space and its value. Parse refuses the syntax;
// Validate, which Parse calls last, refuses the meaning.
func Parse(page []byte) (Meta, []byte, error) {
	var m Meta
	if bytes.Contains(page, []byte("\r")) {
		return m, nil, fmt.Errorf("%w: the page holds a carriage return", ErrInvalid)
	}
	lines := strings.Split(string(page), "\n")
	if lines[0] != fence {
		return m, nil, fmt.Errorf("%w: line 1 is not %q", ErrInvalid, fence)
	}
	seen := -1
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == fence {
			if i+1 == len(lines) {
				return m, nil, fmt.Errorf("%w: the closing %q has no newline after it", ErrInvalid, fence)
			}
			if err := Validate(m); err != nil {
				return Meta{}, nil, err
			}
			return m, []byte(strings.Join(lines[i+1:], "\n")), nil
		}
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			return m, nil, fmt.Errorf("%w: line %d is not a key, a colon, a space and a value", ErrInvalid, i+1)
		}
		at := slices.Index(keys, key)
		switch {
		case at < 0:
			return m, nil, fmt.Errorf("%w: line %d: unknown key %q", ErrInvalid, i+1, key)
		case at == seen:
			return m, nil, fmt.Errorf("%w: line %d: %s is given twice", ErrInvalid, i+1, key)
		case at < seen:
			return m, nil, fmt.Errorf("%w: line %d: %s comes before %s", ErrInvalid, i+1, keys[seen], key)
		}
		seen = at
		if err := set(&m, key, value); err != nil {
			return m, nil, fmt.Errorf("%w: line %d: %w", ErrInvalid, i+1, err)
		}
	}
	return m, nil, fmt.Errorf("%w: no closing %q", ErrInvalid, fence)
}

// HasBlock reports whether a page opens with a block, parsable or not.
func HasBlock(page []byte) bool {
	return bytes.HasPrefix(page, []byte(fence+"\n"))
}

func set(m *Meta, key, value string) error {
	switch key {
	case keyAudience, keyCovers:
		list, err := parseList(value)
		if err != nil {
			return err
		}
		if key == keyAudience {
			m.Audience = list
		} else {
			m.Covers = list
		}
		return nil
	}
	if err := checkScalar(value); err != nil {
		return err
	}
	switch key {
	case keyTitle:
		m.Title = value
	case keySummary:
		m.Summary = value
	case keyType:
		m.Type = value
	case keyGenerated:
		m.Generated = value
	}
	return nil
}

// parseList reads a flow list: [a] or [a, b] with one comma and one space
// between items and no other white space.
func parseList(value string) ([]string, error) {
	inner, ok := strings.CutPrefix(value, "[")
	if !ok {
		return nil, fmt.Errorf("a list starts with [")
	}
	inner, ok = strings.CutSuffix(inner, "]")
	if !ok {
		return nil, fmt.Errorf("a list ends with ]")
	}
	if inner == "" {
		return nil, nil
	}
	items := strings.Split(inner, ", ")
	for i, item := range items {
		if err := checkItem(item); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
	}
	return items, nil
}
