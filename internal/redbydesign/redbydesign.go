// Package redbydesign reads the list of scenarios that stay red on a recorded
// finding in a system under test. The runner judges a catalogue run against
// it, and a lab check proves every listed scenario exists, so both read the
// file the same way.
//
// One scenario per line: its identifier, whitespace, the finding in one line.
// A line starting with # and a blank line are skipped.
package redbydesign

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// Entry is one listed scenario and the finding that keeps it red.
type Entry struct {
	ID      string
	Finding string
}

// identifier is a scenario's file name without its extension; a path or a
// file name would never match the identifier a run reports.
var identifier = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Read parses the list at path. A missing file is an error, never an empty list.
func Read(path string) ([]Entry, error) {
	file, err := os.Open(path) // #nosec G304 -- the list the caller names.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return Parse(file, path)
}

// Parse reads a list; name is used in its errors.
func Parse(r io.Reader, name string) ([]Entry, error) {
	var entries []Entry
	seen := map[string]int{}
	scanner := bufio.NewScanner(r)
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, number, err)
		}
		if first, twice := seen[entry.ID]; twice {
			return nil, fmt.Errorf("%s:%d: %s is listed on line %d already", name, number, entry.ID, first)
		}
		seen[entry.ID] = number
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return entries, nil
}

func parseLine(line string) (Entry, error) {
	id, finding := line, ""
	if at := strings.IndexFunc(line, unicode.IsSpace); at >= 0 {
		id, finding = line[:at], strings.TrimSpace(line[at:])
	}
	switch {
	case !identifier.MatchString(id):
		return Entry{}, fmt.Errorf("%q is not a scenario identifier (its file name without .yaml)", id)
	case finding == "":
		return Entry{}, fmt.Errorf("%s names no finding; say in one line what keeps it red", id)
	}
	return Entry{ID: id, Finding: finding}, nil
}
