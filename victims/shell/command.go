package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A command that hangs would hold a scenario open until the runner gave up on
// the whole run. Nothing here blocks on its own, but cat reads a file the
// caller named and the size of that file is the caller's choice.
const commandDeadline = 10 * time.Second

var (
	errNoCommand = errors.New("no command")
	errNotOnList = errors.New("program is not on the allowlist")
)

// outcome is what one command left behind. ExitCode is reported rather than
// turned into an error: a command that ran and failed is still a command that
// ran, and journalling it as refused would let a scenario read a failed effect
// as an effect that never happened.
type outcome struct {
	Program  string   `json:"program"`
	Args     []string `json:"args"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr"`
	ExitCode int      `json:"exit_code"`
}

// split is the whole of this server's command parsing. It splits on spaces and
// knows nothing about quoting, pipes or redirection; it is a victim, not a
// shell.
func split(table map[string]program, command string) (string, []string, error) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", nil, errNoCommand
	}
	if _, ok := table[fields[0]]; !ok {
		return "", nil, fmt.Errorf("%w: %s", errNotOnList, fields[0])
	}
	return fields[0], fields[1:], nil
}

// run answers with an error only when the command did not run at all: an empty
// line, a name the allowlist does not carry, or a caller that is already gone.
// Those are the refusals; everything else ran, and an exit code says how it
// went.
func run(ctx context.Context, command string) (outcome, error) {
	table := programs()
	name, args, err := split(table, command)
	if err != nil {
		return outcome{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, commandDeadline)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return outcome{}, err
	}

	wrote := table[name](ctx, args)
	return outcome{
		Program:  name,
		Args:     args,
		Stdout:   wrote.stdout,
		Stderr:   wrote.stderr,
		ExitCode: wrote.exitCode,
	}, nil
}
