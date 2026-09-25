package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/guardana/playground/internal/assertion"
)

// Compose is the part of docker compose the runner uses.
//
// It is an interface so the orchestration around it can be run end to end
// without docker: what the runner does with a service that did not start is the
// part that decides whether a red run is reported as red, and that has to be
// testable. The implementation below is kept to the command building and the
// output parsing, both of which a reader can check by eye.
type Compose interface {
	// Services names every service in the profiles, whether or not it is up.
	Services(ctx context.Context, profiles []string) ([]string, error)
	// Build builds every image the profiles use, the one-shots included; Up
	// and RunOnce build nothing.
	Build(ctx context.Context, profiles []string) error
	Up(ctx context.Context, profiles, services []string) error
	Status(ctx context.Context, profiles, services []string) ([]assertion.Service, error)
	// RunOnce runs one service to completion with the arguments given. The
	// error is for a command that could not be run at all; a container that ran
	// and exited non-zero comes back in the Execution.
	RunOnce(ctx context.Context, profiles []string, service string, args []string) (Execution, error)
	// RunSplit is RunOnce for a service whose standard output is a record: it
	// comes back apart from standard error, which carries compose's own
	// progress. A non-empty entrypoint replaces the image's. It builds
	// nothing: the verifier services it runs are built by `make
	// verifier-image` alone, so the image the runner checked is the one that runs.
	RunSplit(ctx context.Context, profiles []string, service, entrypoint string, args []string) (Split, error)
	// Exec runs a command inside a running service's container, its output
	// kept apart from compose's own.
	Exec(ctx context.Context, profiles []string, service string, args []string) (Split, error)
	// Logs is what a service printed, its output streams merged.
	Logs(ctx context.Context, profiles []string, service string) (Split, error)
	// Stop stops one service and waits for it to exit; a service that flushes
	// on SIGTERM has flushed when it returns without error.
	Stop(ctx context.Context, profiles []string, service string) error
	// ContainerImage is the image ID the project's own container of service
	// runs, whatever tag it was started from.
	ContainerImage(ctx context.Context, profiles []string, service string) (string, error)
	Down(ctx context.Context, profiles []string) error
	// WithEnv returns a Compose that adds these variables to every invocation.
	// The run identifier is one of them, and it changes per run, so it is not
	// something the topology can be built with once.
	WithEnv(env map[string]string) Compose
}

// Execution is what one container did.
type Execution struct {
	ExitCode int
	Output   string
}

// Split is what one container did, its two output streams kept apart.
type Split struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// dockerCompose runs the real thing.
type dockerCompose struct {
	file      string
	envFile   string
	directory string
	// env is added to the runner's own environment for every invocation, which
	// is how compose interpolates ${LAB_RUN_ID} and the rest.
	env []string
	log io.Writer
}

func (d dockerCompose) WithEnv(env map[string]string) Compose {
	added := slices.Clone(os.Environ())
	for _, name := range slices.Sorted(maps.Keys(env)) {
		added = append(added, name+"="+env[name])
	}
	d.env = added
	return d
}

func (d dockerCompose) Services(ctx context.Context, profiles []string) ([]string, error) {
	output, err := d.capture(ctx, profiles, "config", "--services")
	if err != nil {
		return nil, err
	}
	var services []string
	for _, line := range strings.Split(output, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			services = append(services, name)
		}
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no service in profiles %s", strings.Join(profiles, ", "))
	}
	slices.Sort(services)
	return services, nil
}

func (d dockerCompose) Build(ctx context.Context, profiles []string) error {
	_, err := d.capture(ctx, profiles, "build")
	return err
}

func (d dockerCompose) Up(ctx context.Context, profiles, services []string) error {
	_, err := d.capture(ctx, profiles, upArgs(services)...)
	return err
}

func (d dockerCompose) Status(ctx context.Context, profiles, services []string) ([]assertion.Service, error) {
	output, err := d.capture(ctx, profiles, append([]string{"ps", "--all", "--format", "json"}, services...)...)
	if err != nil {
		return nil, err
	}
	return parseStatus(output, services)
}

func (d dockerCompose) RunOnce(ctx context.Context, profiles []string, service string, args []string) (Execution, error) {
	command := d.command(ctx, profiles, runOnceArgs(service, args)...)
	output, err := command.CombinedOutput()
	execution := Execution{Output: string(output)}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		execution.ExitCode = exit.ExitCode()
	default:
		return execution, err
	}
	return execution, nil
}

func (d dockerCompose) RunSplit(
	ctx context.Context, profiles []string, service, entrypoint string, args []string,
) (Split, error) {
	return d.split(d.command(ctx, profiles, runSplitArgs(service, entrypoint, args)...))
}

func (d dockerCompose) Exec(ctx context.Context, profiles []string, service string, args []string) (Split, error) {
	return d.split(d.command(ctx, profiles, append([]string{"exec", "-T", service}, args...)...))
}

func (d dockerCompose) Logs(ctx context.Context, profiles []string, service string) (Split, error) {
	return d.split(d.command(ctx, profiles, "logs", "--no-color", "--no-log-prefix", service))
}

func (d dockerCompose) split(command *exec.Cmd) (Split, error) {
	return splitWithin(command, maxStreamBytes)
}

// maxStreamBytes bounds each stream a split run keeps: a container that prints
// without end would otherwise hold the runner's memory, and a report cut at an
// arbitrary byte is not the report the container wrote.
const maxStreamBytes = 16 << 20

func splitWithin(command *exec.Cmd, limit int) (Split, error) {
	out, problems := &boundedBuffer{limit: limit}, &boundedBuffer{limit: limit}
	command.Stdout, command.Stderr = out, problems
	err := command.Run()
	split := Split{Stdout: out.String(), Stderr: problems.String()}
	if out.over || problems.over {
		return split, fmt.Errorf("the container printed more than %d bytes on one stream", limit)
	}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		split.ExitCode = exit.ExitCode()
	default:
		return split, err
	}
	return split, nil
}

// boundedBuffer refuses a write past its limit, which stops the copy and
// closes the pipe the process writes into.
type boundedBuffer struct {
	strings.Builder
	limit int
	over  bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.over = true
		return 0, errors.New("stream past its bound")
	}
	return b.Builder.Write(p)
}

func (d dockerCompose) Stop(ctx context.Context, profiles []string, service string) error {
	_, err := d.capture(ctx, profiles, "stop", service)
	return err
}

func (d dockerCompose) Down(ctx context.Context, profiles []string) error {
	_, err := d.capture(ctx, profiles, "down", "--volumes", "--remove-orphans")
	return err
}

func (d dockerCompose) capture(ctx context.Context, profiles []string, args ...string) (string, error) {
	command := d.command(ctx, profiles, args...)
	var out, problems strings.Builder
	command.Stdout = &out
	command.Stderr = &problems
	if err := command.Run(); err != nil {
		return out.String(), fmt.Errorf("docker compose %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(problems.String()))
	}
	if d.log != nil && problems.Len() > 0 {
		_, _ = io.WriteString(d.log, problems.String())
	}
	return out.String(), nil
}

func (d dockerCompose) command(ctx context.Context, profiles []string, args ...string) *exec.Cmd {
	// #nosec G204,G702 -- the arguments are built by this package from the
	// compose topology and the scenario, which is the clone's or the workspace
	// of the person running the lab; no shell reads them and each is one argument.
	command := exec.CommandContext(ctx, "docker", append([]string{"compose"},
		composeArgs(d.file, d.envFile, profiles, args)...)...)
	command.Dir = d.directory
	command.Env = d.env
	return command
}
