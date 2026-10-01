// Package redbydesign reads the list of scenarios that stay red on a recorded
// finding in a system under test. The runner judges a catalogue run against
// it, and a lab check proves every listed scenario exists, so both read the
// file the same way.
//
// One scenario per line: its identifier, whitespace, the checks it fails on
// separated by commas, whitespace, the finding in one line. A line starting
// with # and a blank line are skipped.
package redbydesign

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Entry is one listed scenario, the checks it fails on, and the finding that
// keeps it red.
type Entry struct {
	ID      string
	Checks  []string
	Finding string
}

// identifier is a scenario's file name without its extension; a path or a
// file name would never match the identifier a run reports.
var identifier = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// checkID is a check identifier as a result carries it: a family, then at
// least one further part. A bare word is refused, so a finding whose first
// word slipped into the check field cannot parse as a check.
var checkID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(/[A-Za-z0-9._-]+)+$`)

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
	fields := strings.Fields(line)
	id := fields[0]
	if !identifier.MatchString(id) {
		return Entry{}, fmt.Errorf("%q is not a scenario identifier (its file name without .yaml)", id)
	}
	if len(fields) < 2 {
		return Entry{}, fmt.Errorf("%s names no check; list the checks it fails on, separated by commas", id)
	}
	checks, err := parseChecks(fields[1])
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", id, err)
	}
	if len(fields) < 3 {
		return Entry{}, fmt.Errorf("%s names no finding; say in one line what keeps it red", id)
	}
	rest := strings.TrimSpace(line[len(id):])
	finding := strings.TrimSpace(rest[len(fields[1]):])
	return Entry{ID: id, Checks: checks, Finding: finding}, nil
}

func parseChecks(field string) ([]string, error) {
	checks := strings.Split(field, ",")
	seen := map[string]bool{}
	for _, check := range checks {
		switch {
		case check == "":
			return nil, fmt.Errorf("%q holds an empty check", field)
		case !checkID.MatchString(check):
			return nil, fmt.Errorf("%q is not a check identifier as a result carries it (family/name)", check)
		case seen[check]:
			return nil, fmt.Errorf("%s is named twice", check)
		}
		seen[check] = true
	}
	return checks, nil
}
