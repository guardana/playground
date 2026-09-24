package main

import "time"

// How long a run may take. AGENTS.md: anything crossing I/O takes a context and
// carries a deadline, and the runner is the process that has to survive to
// write the report. A `docker compose up --wait` against a healthcheck that
// never passes, or a `run` against a container that hangs, would otherwise wedge
// one scenario with no junit.xml and no report.md — and under -all, every
// scenario after it.
const (
	// defaultScenarioTimeout bounds one scenario, the first docker build
	// included. The slow part is `up --build` with no layer cache, which builds
	// every image in the profile. The bound is loose on purpose: it is here to
	// stop a wedged run from wedging the catalogue, not to time a build, and a
	// scenario that runs past it is not slow but stuck. -timeout raises it for a
	// machine or a network that needs longer.
	defaultScenarioTimeout = 20 * time.Minute

	// teardownTimeout bounds taking the profile down. It is separate from the
	// scenario's own deadline because the run that just ran out of time is the
	// run whose containers are still up. Stopping them is seconds of work:
	// compose sends SIGTERM and waits out one grace period per service.
	teardownTimeout = 3 * time.Minute
)

// deadline is how long this lab gives one scenario. A lab built without a
// timeout still gets the default: the rule is that every docker call carries a
// deadline, and a zero here would be one that has already passed.
func (l lab) deadline() time.Duration {
	if l.timeout <= 0 {
		return defaultScenarioTimeout
	}
	return l.timeout
}
