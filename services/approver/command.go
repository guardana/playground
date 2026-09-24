package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// maxOutputBytes bounds what one run of the command may print. A listing past
// it is refused rather than parsed short.
const maxOutputBytes = 8 << 20

// commander runs the enforcer's own command. It is the only writer of the
// approvals directory; the approver only chooses its arguments.
type commander struct {
	path string
	// env is added to this process's environment for the child, and is nil
	// outside a test.
	env     []string
	timeout time.Duration
}

// outcome is what one run of the command came to.
type outcome struct {
	stdout, stderr []byte
	// exited is false when the command did not run to an exit of its own:
	// it could not start, or it was killed at its deadline.
	exited bool
	// cutShort is true when the command was killed after it started: at its
	// deadline, at shutdown, or by a signal. It may have written already.
	cutShort bool
	code     int
	err      error
}

// status is the exit status as a journal line spells it.
func (o outcome) status() string {
	if !o.exited {
		return "none"
	}
	return strconv.Itoa(o.code)
}

func (c commander) run(ctx context.Context, args ...string) outcome {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.path, args...) // #nosec G204 G702 -- the enforcer's command, with arguments the approver built.
	if c.env != nil {
		cmd.Env = append(os.Environ(), c.env...)
	}
	cmd.WaitDelay = time.Second
	stdout, stderr := &capped{}, &capped{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	o := outcome{stdout: stdout.buf, stderr: stderr.buf, err: err}
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		o.err, o.cutShort = errors.Join(err, ctx.Err()), cmd.Process != nil
	case err == nil:
		o.exited = true
	case errors.As(err, &exit) && exit.Exited():
		o.exited, o.code = true, exit.ExitCode()
	case errors.As(err, &exit):
		o.cutShort = true
	}
	if o.err == nil && (stdout.over || stderr.over) {
		o.err = errors.New("the command printed more than the approver reads")
	}
	return o
}

// capped keeps the first maxOutputBytes written to it and notes anything past.
type capped struct {
	buf  []byte
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	n := len(p)
	room := maxOutputBytes - len(c.buf)
	if len(p) > room {
		c.over = true
		p = p[:max(room, 0)]
	}
	c.buf = append(c.buf, p...)
	return n, nil
}
