// Package control drives the enforcer as the gate in front of the victims: it
// prepares the enforcer's signed configuration for a run, settles and reads
// the trail it exports, and grades the run from that trail.
package control

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/guardana/playground/runner/driver"
)

// versionFile is where the lab pins the enforcer it tests.
const versionFile = "versions.env"

// Workspace is where a run reads the files a scenario names.
type Workspace struct {
	// Dir is absolute with every link resolved.
	Dir string
	// External is set when Dir is not the clone.
	External bool
}

func (w Workspace) file(named string) string { return filepath.Join(w.Dir, filepath.FromSlash(named)) }

// Config is what the lab hands the enforcer's driver once, for every run.
type Config struct {
	// Root is the clone, which the lab's own files are read from.
	Root      string
	Reports   string
	Workspace Workspace
	// Inspect runs docker on the host: it reads images and signs policies.
	Inspect driver.Lookup
	Log     io.Writer
	// Clock dates the collector's certificates; nil is time.Now.
	Clock func() time.Time
	// KeysDir holds the lab key; Sign signs a scenario's policy with it, and
	// nil signs with the pinned enforcer's own command.
	KeysDir string
	Sign    Signer
	// DrainBound bounds the wait for the trail; zero is drainTimeout.
	DrainBound time.Duration
	// Pins are versions.env's, which name the enforcer's image, commit and
	// namespace.
	Pins driver.Pins
	// Development names a development image of the enforcer to run instead of
	// the pinned one; empty runs the pin.
	Development string
}

// Driver is the enforcer, built once from its Config.
type Driver struct {
	config Config
	sign   Signer
	// namespace is the one under which the enforcer's gateway marks the
	// answers it makes itself; the agent reads a pending answer only under it.
	namespace string
	// commit is ENFORCER_COMMIT; pin is the version the running enforcer has
	// to report: the commit, or a development build's own.
	commit, pin string
	// image is ENFORCER_IMAGE:ENFORCER_COMMIT, or the development image used
	// instead.
	image       string
	development *devBuild
}

// New reads the enforcer's pins and, for a development build, the image that
// replaces them, which it refuses when the development build did not make it.
func New(ctx context.Context, config Config) (*Driver, error) {
	built := &Driver{config: config, sign: config.Sign}
	var err error
	if built.namespace, err = enforcerNamespace(config.Pins); err != nil {
		return nil, err
	}
	if built.commit, err = enforcerPin(config.Pins); err != nil {
		return nil, err
	}
	built.pin = built.commit
	if built.image, err = enforcerImage(config.Pins); err != nil {
		return nil, err
	}
	if built.sign == nil {
		built.sign = signWithEnforcer(config.Root, config.Inspect)
	}
	if config.Development == "" {
		return built, nil
	}
	dev, err := readDevelopment(ctx, config.Inspect, config.Development)
	if err != nil {
		return nil, err
	}
	built.pin, built.image, built.development = dev.Version, dev.Ref, &dev
	built.sign = signWithImage(dev.Ref, config.Inspect)
	return built, nil
}

// Config is what the driver was built from.
func (d *Driver) Config() Config { return d.config }

// Development names the development build the driver runs in place of the
// pin, in one line; empty at the pin.
func (d *Driver) Development() string {
	if d.development == nil {
		return ""
	}
	return d.development.describe(d.commit)
}

// enforcerNamespace is the namespace the pins name for the enforcer. Its
// gateway marks the answers it makes itself under it, and the agent is told it
// so an upstream result shaped like a pending answer is not retried as one.
func enforcerNamespace(pins driver.Pins) (string, error) {
	namespace := pins.Value("ENFORCER_NAMESPACE")
	if namespace == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_NAMESPACE", versionFile)
	}
	return namespace, nil
}

// enforcerPin is the commit the pins name for the enforcer.
func enforcerPin(pins driver.Pins) (string, error) {
	pin := pins.Value("ENFORCER_COMMIT")
	if pin == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_COMMIT", versionFile)
	}
	return pin, nil
}

// enforcerImage is the enforcer's image as the pins name it, name and tag.
func enforcerImage(pins driver.Pins) (string, error) {
	name, commit := pins.Value("ENFORCER_IMAGE"), pins.Value("ENFORCER_COMMIT")
	if name == "" || commit == "" {
		return "", fmt.Errorf("%s pins no ENFORCER_IMAGE or no ENFORCER_COMMIT", versionFile)
	}
	return name + ":" + commit, nil
}
