package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// scenarioDir is where a scenario lives, one directory per family.
const scenarioDir = "scenarios"

// settings is the whole of the runner's command line.
type settings struct {
	scenario string
	all      bool
	reports  string
	keep     bool
	timeout  time.Duration
}

// locate returns the scenario files to run, in the order they will be run.
//
// -scenario takes either an identifier, which is a file's own name, or a path.
// An identifier two files claim is refused rather than resolved to the first
// one: a run has to report under a name that names one thing.
func locate(root string, s settings) ([]string, error) {
	if s.all {
		if s.scenario != "" {
			// Silently ignoring one of them would run something other than what
			// the command line asked for.
			return nil, errors.New("-all and -scenario ask for different runs; give one of them")
		}
		return everyScenario(root)
	}
	if s.scenario == "" {
		return nil, errors.New("one of -scenario or -all is required")
	}
	if looksLikeAPath(s.scenario) {
		if _, err := os.Stat(s.scenario); err != nil {
			return nil, fmt.Errorf("-scenario %s: %w", s.scenario, err)
		}
		return []string{s.scenario}, nil
	}
	pattern := filepath.Join(root, scenarioDir, "*", s.scenario+".yaml")
	found, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no scenario named %q under %s", s.scenario, filepath.Join(root, scenarioDir))
	case 1:
		return found, nil
	default:
		return nil, fmt.Errorf("two or more scenarios are named %q: %s", s.scenario, strings.Join(found, ", "))
	}
}

func everyScenario(root string) ([]string, error) {
	found, err := filepath.Glob(filepath.Join(root, scenarioDir, "*", "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		// A run over nothing reports that every scenario passed, which is the
		// one answer a lab must never give by accident.
		return nil, fmt.Errorf("no scenario found under %s", filepath.Join(root, scenarioDir))
	}
	slices.Sort(found)
	return found, nil
}

func looksLikeAPath(value string) bool {
	return strings.ContainsRune(value, filepath.Separator) ||
		strings.HasSuffix(value, ".yaml") ||
		strings.HasSuffix(value, ".yml")
}
