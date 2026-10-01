// Command runner runs a scenario against the lab and grades it from the
// records the run left behind.
//
//	runner -scenario <id or path> [-reports dir] [-keep] [-timeout d]
//	runner -all [-reports dir] [-timeout d] [-red-by-design file]
//	runner ... -enforcer-dev <image>
//
// It exits zero only when every scenario it ran passed, or, with
// -red-by-design, when the ones that did not are exactly those the file lists,
// each failing on a record. Anything else — a service that did not start, a
// step with no decision, a check that could not read its input — is a run that
// did not establish what it claimed, and a lab that reported those as green
// would be worth nothing.
//
// The lab's own files are read relative to the working directory, which is
// the clone: `make scenario` runs it from there. A scenario names its files by
// their paths in the workspace, which is the clone too unless LAB_WORKSPACE
// names a directory outside it laid out the same way; then scenarios are
// located and read only there.
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
	"github.com/guardana/playground/internal/redbydesign"
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
	if err := refuseServiceUID(os.Getuid()); err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := refuseOverriddenPins(root, environ); err != nil {
		return err
	}
	space, err := openWorkspace(root, chosen.reports, environ)
	if err != nil {
		return err
	}
	scenarios, err := locate(space.dir, chosen)
	if err != nil {
		return err
	}
	listed, err := redByDesignList(chosen)
	if err != nil {
		return err
	}
	subject := newLab(root, space, chosen, environ, out)
	if subject, err = pinEnforcer(subject); err != nil {
		return err
	}
	if chosen.enforcerDev == "" {
		return execute(ctx, subject, scenarios, chosen.redByDesign != "", listed, out)
	}
	if subject, err = withDevelopment(ctx, subject, chosen.enforcerDev, space, command); err != nil {
		return err
	}
	err = execute(ctx, subject, scenarios, chosen.redByDesign != "", listed, out)
	_, _ = fmt.Fprintf(out, "runner: graded against the %s\n", subject.development.describe(subject.pinned))
	return err
}

func execute(
	ctx context.Context, subject lab, scenarios []string, judged bool, listed []redbydesign.Entry, out io.Writer,
) error {
	if judged {
		return executeRedByDesign(ctx, subject, scenarios, listed, out)
	}
	return executeAll(ctx, subject, scenarios, out)
}

// pinEnforcer reads what versions.env pins the enforcer at: its namespace, its
// commit and its image.
func pinEnforcer(subject lab) (lab, error) {
	var err error
	if subject.namespace, err = enforcerNamespace(subject.root); err != nil {
		return lab{}, err
	}
	if subject.pin, err = enforcerPin(subject.root); err != nil {
		return lab{}, err
	}
	if subject.enforcerImage, err = enforcerImage(subject.root); err != nil {
		return lab{}, err
	}
	return subject, nil
}

// withDevelopment points the run at a development image of the enforcer: the
// version, the tree and the image ID it is checked against are the image's
// own, policies are signed with its command, and the report names the build
// and the pin it is not.
func withDevelopment(ctx context.Context, subject lab, ref string, space workspace, run lookup) (lab, error) {
	dev, err := readDevelopment(ctx, run, ref)
	if err != nil {
		return lab{}, err
	}
	subject.pinned = subject.pin
	subject.pin, subject.enforcerImage, subject.development = dev.Version, dev.Ref, &dev
	subject.describe = describeHost(subject.root, space, run, &dev)
	subject.sign = signWithImage(dev.Ref, run)
	return subject, nil
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
	set.StringVar(&chosen.redByDesign, "red-by-design", "",
		"with -all, pass only when the scenarios that do not pass are exactly the ones this file lists")
	set.StringVar(&chosen.enforcerDev, "enforcer-dev", "",
		"run this development image of the enforcer (scripts/build-enforcer-dev.sh) instead of the pinned one")
	if err := set.Parse(args); err != nil {
		return settings{}, err
	}
	var empty error
	set.Visit(func(given *flag.Flag) {
		if given.Name == "enforcer-dev" && chosen.enforcerDev == "" {
			empty = errors.New("-enforcer-dev names no image; a run without one is a run at the pin")
		}
	})
	return chosen, empty
}

func newLab(root string, space workspace, chosen settings, environ []string, out io.Writer) lab {
	return lab{
		root:      root,
		workspace: space,
		reports:   chosen.reports,
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
		describe: describeHost(root, space, command, nil),
		keysDir:  labKeysDir(lookupIn(environ)),
		sign:     signWithEnforcer(root, command),
		inspect:  command,
	}
}

// executeAll runs every scenario and reports a failure if any of them did not
// pass. It runs them all before returning: stopping at the first red would hide
// the rest, and a person looking at a broken lab wants the whole picture.
func executeAll(ctx context.Context, subject lab, scenarios []string, out io.Writer) error {
	var failed []string
	for _, ran := range runEach(ctx, subject, scenarios, out) {
		if ran.outcome != assertion.Pass {
			failed = append(failed, ran.id)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d scenarios did not pass: %s",
			len(failed), len(scenarios), strings.Join(failed, ", "))
	}
	return nil
}

// runEach runs every scenario and prints one line for each. A run that could
// not be written established nothing and counts as indeterminate.
func runEach(ctx context.Context, subject lab, scenarios []string, out io.Writer) []ranScenario {
	ran := make([]ranScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		graded, err := subject.execute(ctx, scenario)
		if err != nil {
			id := strings.TrimSuffix(filepath.Base(scenario), filepath.Ext(scenario))
			ran = append(ran, ranScenario{id: id, outcome: assertion.Indeterminate})
			_, _ = fmt.Fprintf(out, "%s: the run could not be written: %v\n", filepath.Base(scenario), err)
			continue
		}
		suite := graded.Suite()
		if subject.development != nil {
			suite = "dev-" + suite
		}
		_, _ = fmt.Fprintf(out, "%-10s %-9s %s  %s\n", graded.Outcome(), suite, graded.Scenario,
			filepath.Join(subject.reports, graded.RunID, "report.md"))
		ran = append(ran, ranScenario{id: graded.Scenario, outcome: graded.Outcome()})
	}
	return ran
}

// randomSuffix keeps two runs of one scenario in one second apart. The error is
// dropped because crypto/rand.Read does not return one, and a collision is
// caught anyway: the runner refuses a run directory that already exists.
func randomSuffix() string {
	var bytes [4]byte
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}
