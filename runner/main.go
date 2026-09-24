// Command runner runs a scenario against the lab and grades it from the
// records the run left behind.
//
//	runner -scenario <id or path> [-reports dir] [-keep] [-timeout d]
//	runner -all [-reports dir] [-timeout d]
//
// It exits zero only when every scenario it ran passed. Anything else — a
// service that did not start, a step with no decision, a check that could not
// read its input — is a run that did not establish what it claimed, and a lab
// that reported those as green would be worth nothing.
//
// Paths are read relative to the working directory, which is the repository
// root: `make scenario` runs it from there, and a scenario names its trajectory
// and its declared verdicts by their paths in the repository.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guardana/playground/internal/assertion"
)

const (
	composeFile = "compose/compose.yaml"
	versionFile = "versions.env"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Environ(), os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "runner: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args, environ []string, out io.Writer) error {
	chosen, err := parse(args, out)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return nil
	case err != nil:
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := refuseOverriddenPins(root, environ); err != nil {
		return err
	}
	scenarios, err := locate(root, chosen)
	if err != nil {
		return err
	}
	return executeAll(ctx, newLab(root, chosen, environ, out), scenarios, out)
}

func parse(args []string, out io.Writer) (settings, error) {
	var chosen settings
	set := flag.NewFlagSet("runner", flag.ContinueOnError)
	set.SetOutput(out)
	set.StringVar(&chosen.scenario, "scenario", "", "the scenario to run, by identifier or by path")
	set.BoolVar(&chosen.all, "all", false, "run every scenario in the catalogue")
	set.StringVar(&chosen.reports, "reports", "reports", "where to write each run's records and reports")
	set.BoolVar(&chosen.keep, "keep", false, "leave the profile up after the run, for looking at it by hand")
	set.DurationVar(&chosen.timeout, "timeout", defaultScenarioTimeout,
		"how long one scenario may take, the first docker build included")
	if err := set.Parse(args); err != nil {
		return settings{}, err
	}
	return chosen, nil
}

func newLab(root string, chosen settings, environ []string, out io.Writer) lab {
	return lab{
		root:    root,
		reports: chosen.reports,
		compose: dockerCompose{
			file:      composeFile,
			envFile:   versionFile,
			directory: root,
			env:       environ,
			log:       out,
		},
		timeout:  chosen.timeout,
		keep:     chosen.keep,
		clock:    time.Now,
		suffix:   randomSuffix,
		log:      out,
		describe: describeHost(root, command),
	}
}

// executeAll runs every scenario and reports a failure if any of them did not
// pass. It runs them all before returning: stopping at the first red would hide
// the rest, and a person looking at a broken lab wants the whole picture.
func executeAll(ctx context.Context, subject lab, scenarios []string, out io.Writer) error {
	var failed []string
	for _, scenario := range scenarios {
		graded, err := subject.execute(ctx, scenario)
		if err != nil {
			failed = append(failed, filepath.Base(scenario))
			_, _ = fmt.Fprintf(out, "%s: the run could not be written: %v\n", filepath.Base(scenario), err)
			continue
		}
		_, _ = fmt.Fprintf(out, "%-10s %s  %s\n", graded.Outcome(), graded.Scenario,
			filepath.Join(subject.reports, graded.RunID, "report.md"))
		if graded.Outcome() != assertion.Pass {
			failed = append(failed, graded.Scenario)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d scenarios did not pass: %s",
			len(failed), len(scenarios), strings.Join(failed, ", "))
	}
	return nil
}

// randomSuffix keeps two runs of one scenario in one second apart. The error is
// dropped because crypto/rand.Read does not return one, and a collision is
// caught anyway: the runner refuses a run directory that already exists.
func randomSuffix() string {
	var bytes [4]byte
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}
