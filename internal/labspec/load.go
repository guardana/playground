// Package labspec holds the two files a scenario run is written in: the
// trajectory that says which tool calls happen, and the scenario that says what
// the lab expects to find afterwards. Both shapes are frozen here so that the
// agent, the runner and the services agree on one reading of them.
//
// Loading is strict in both directions. A key this package does not know is
// refused rather than dropped, because a misspelled expectation that loads is
// an assertion nobody makes; and every rule that can be checked without running
// anything is checked at load, so a scenario fails at the point where the file
// is in front of the person who wrote it.
//
// Steps are numbered from 1, in the trajectory's references and in the
// scenario's expectations alike. Two numbering schemes over one list is a
// reading error waiting to happen in the file that decides whether a run
// passed.
package labspec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// ErrInvalid reports a file this package will not run on: a version it does not
// implement, a value outside the set the field allows, or a rule broken between
// the two files. The message names the field and what is wrong with it.
var ErrInvalid = errors.New("labspec: invalid")

// SchemaVersion is the only version this package reads. A file carrying another
// is refused and never read under this one's rules.
const SchemaVersion = 1

// MaxFileBytes bounds one trajectory or scenario file. These are written by
// hand and read into memory whole; the bound is what keeps a generated or
// truncated file from being an allocation before a single field is checked.
const MaxFileBytes = 256 << 10

// LoadTrajectory reads one trajectory file and checks every rule that does not
// need the scenario: the version, the required fields, and that each step
// reference names an earlier step.
func LoadTrajectory(path string) (Trajectory, error) {
	var trajectory Trajectory
	if err := readStrict(path, &trajectory); err != nil {
		return Trajectory{}, err
	}
	if err := trajectory.validate(); err != nil {
		return Trajectory{}, fmt.Errorf("%s: %w", path, err)
	}
	return trajectory, nil
}

// LoadScenario reads one scenario file and checks every rule that does not need
// the trajectory. The identifier has to be the file's own name: a scenario
// copied to a new file and left with the old identifier would report under a
// name nothing on disk carries.
func LoadScenario(path string) (Scenario, error) {
	var scenario Scenario
	if err := readStrict(path, &scenario); err != nil {
		return Scenario{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if err := first(scenario.validate(name), validateGap(scenario, Class(path))); err != nil {
		return Scenario{}, fmt.Errorf("%s: %w", path, err)
	}
	return scenario, nil
}

// readStrict reads path as YAML into value, refusing a key value has no field
// for. sigs.k8s.io/yaml converts the document to JSON first, so the json tags
// on these types are the whole mapping and there is no second set to keep in
// step with them.
func readStrict(path string, value any) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > MaxFileBytes {
		return fmt.Errorf("%s: %w: %d bytes, limit %d", path, ErrInvalid, info.Size(), MaxFileBytes)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- the path is the file the caller asked for.
	if err != nil {
		return err
	}
	if err := yaml.UnmarshalStrict(body, value); err != nil {
		return fmt.Errorf("%s: %w: %w", path, ErrInvalid, err)
	}
	return nil
}

// oneOf reports a value outside the set the field allows, naming the set so the
// message is enough to fix the file without opening this package.
func oneOf(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%w: %s is %q, want one of %s", ErrInvalid, field, value, strings.Join(allowed, ", "))
}

func required(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalid, field)
	}
	return nil
}

// first returns the first problem in the order the checks were written, so a
// caller reads one message about the field nearest the top of the file rather
// than a list it has to work through backwards.
func first(checks ...error) error {
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}
