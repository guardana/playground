package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// errInvalidConfig reports flags or an environment this service will not start on.
var errInvalidConfig = errors.New("approver: invalid configuration")

// settings is what the flags and the lab environment say about one run.
type settings struct {
	control     string
	dir         string
	script      string
	listen      string
	interval    time.Duration
	execTimeout time.Duration
	runID       string
	reportsDir  string
}

// parseSettings reads the flags in args and the environment through lookup,
// which is os.LookupEnv outside a test. -help prints to usage and returns
// flag.ErrHelp.
func parseSettings(args []string, lookup func(string) (string, bool), usage io.Writer) (settings, error) {
	flags := flag.NewFlagSet(serverName, flag.ContinueOnError)
	flags.SetOutput(usage)
	var s settings
	flags.StringVar(&s.control, "control", "/enforcer/control", "the enforcer's control command, which alone writes the approvals directory")
	flags.StringVar(&s.dir, "dir", "", "the approvals directory the plane holds")
	flags.StringVar(&s.script, "script", "", "YAML file scripting the answers")
	flags.StringVar(&s.listen, "listen", ":8080", "address /healthz is served on")
	flags.DurationVar(&s.interval, "interval", 250*time.Millisecond, "how often the directory is listed")
	flags.DurationVar(&s.execTimeout, "exec-timeout", 10*time.Second, "deadline for one run of the command")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return settings{}, err
		}
		return settings{}, fmt.Errorf("%w: %w", errInvalidConfig, err)
	}
	if flags.NArg() != 0 {
		return settings{}, fmt.Errorf("%w: unexpected argument %q", errInvalidConfig, flags.Arg(0))
	}
	for name, value := range map[string]string{"-control": s.control, "-dir": s.dir, "-script": s.script, "-listen": s.listen} {
		if strings.TrimSpace(value) == "" {
			return settings{}, fmt.Errorf("%w: %s is empty", errInvalidConfig, name)
		}
	}
	if s.interval <= 0 || s.execTimeout <= 0 {
		return settings{}, fmt.Errorf("%w: -interval and -exec-timeout must be positive", errInvalidConfig)
	}
	if err := s.readEnv(lookup); err != nil {
		return settings{}, err
	}
	return s, nil
}

func (s *settings) readEnv(lookup func(string) (string, bool)) error {
	for _, target := range []struct {
		name string
		into *string
	}{{"LAB_RUN_ID", &s.runID}, {"LAB_REPORTS_DIR", &s.reportsDir}} {
		value, _ := lookup(target.name)
		if strings.TrimSpace(value) == "" {
			// A default would journal somewhere the runner does not read, and
			// a missing journal must stay distinguishable from an empty one.
			return fmt.Errorf("%w: %s is not set", errInvalidConfig, target.name)
		}
		*target.into = strings.TrimSpace(value)
	}
	if strings.ContainsAny(s.runID, `/\`) || strings.Contains(s.runID, "..") {
		return fmt.Errorf("%w: LAB_RUN_ID %q would put the journal outside LAB_REPORTS_DIR", errInvalidConfig, s.runID)
	}
	return nil
}

func (s settings) journalPath() string {
	return filepath.Join(s.reportsDir, s.runID, "journals", serverName+".jsonl")
}
