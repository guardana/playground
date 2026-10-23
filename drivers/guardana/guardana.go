// Package guardana drives the verifier: as the prober it runs each step of a
// scenario against the victims, as the grader it analyses the trace the agent
// wrote, and it hands each step's report or pin on as a neutral record.
package guardana

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/guardana/playground/runner/driver"
)

// What the compose topology names for the verifier and the run it sees.
const (
	verifierService = "verifier"
	// verifierMount is where compose binds a step's directories of the run's
	// verifierDir, the only host paths the verifier sees.
	verifierMount = "/lab-run"
	verifierDir   = "verifier"
	servicePort   = "8080"
	// containerReports is where the agent's container sees the run, and
	// agentDir the agent's own directory in it, on the host as in there.
	containerReports = "/reports"
	agentDir         = "agent"
)

// Config is what the lab hands the verifier's driver once, for every run.
type Config struct {
	// Workspace is where a run reads the files a scenario names, absolute
	// with every link resolved.
	Workspace string
	// Inspect runs docker on the host to read the verifier's image; nil
	// refuses every run the verifier takes part in.
	Inspect driver.Lookup
	Log     io.Writer
	// Pins are versions.env's, which name the verifier's image and release.
	Pins driver.Pins
}

// Driver is the verifier, built once from its Config.
type Driver struct{ config Config }

// New builds the verifier's driver.
func New(config Config) *Driver { return &Driver{config: config} }

// Config is what the driver was built from.
func (d *Driver) Config() Config { return d.config }

func (d *Driver) file(named string) string {
	return filepath.Join(d.config.Workspace, filepath.FromSlash(named))
}

func (d *Driver) note(format string, values ...any) {
	if d.config.Log == nil {
		return
	}
	_, _ = fmt.Fprintf(d.config.Log, "runner: "+format+"\n", values...)
}
