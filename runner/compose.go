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
	Up(ctx context.Context, profiles, services []string) error
	Status(ctx context.Context, profiles, services []string) ([]assertion.Service, error)
	// RunOnce runs one service to completion with the arguments given. The
	// error is for a command that could not be run at all; a container that ran
	// and exited non-zero comes back in the Execution.
	RunOnce(ctx context.Context, profiles []string, service string, args []string) (Execution, error)
	// RunSplit is RunOnce for a service whose standard output is a record: it
	// comes back apart from standard error, which carries compose's own
	// progress. A non-empty entrypoint replaces the image's.
	RunSplit(ctx context.Context, profiles []string, service, entrypoint string, args []string) (Split, error)
	// Exec runs a command inside a running service's container, its output
	// kept apart from compose's own.
	Exec(ctx context.Context, profiles []string, service string, args []string) (Split, error)
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

func (d dockerCompose) Up(ctx context.Context, profiles, services []string) error {
	_, err := d.capture(ctx, profiles, append([]string{"up", "-d", "--build", "--wait"}, services...)...)
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
	// Built apart from the run: compose prints build progress on standard
	// output, which here is the record.
	if _, err := d.capture(ctx, profiles, "build", service); err != nil {
		return Split{}, err
	}
	return d.split(d.command(ctx, profiles, runSplitArgs(service, entrypoint, args)...))
}

func (d dockerCompose) Exec(ctx context.Context, profiles []string, service string, args []string) (Split, error) {
	return d.split(d.command(ctx, profiles, append([]string{"exec", "-T", service}, args...)...))
}

func (d dockerCompose) split(command *exec.Cmd) (Split, error) {
	var out, problems strings.Builder
	command.Stdout, command.Stderr = &out, &problems
	err := command.Run()
	split := Split{Stdout: out.String(), Stderr: problems.String()}
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
	// #nosec G204 -- the arguments are built by this package from the scenario
	// and the compose topology, both of which are the lab's own files.
	command := exec.CommandContext(ctx, "docker", append([]string{"compose"},
		composeArgs(d.file, d.envFile, profiles, args)...)...)
	command.Dir = d.directory
	command.Env = d.env
	return command
}

// composeArgs builds the arguments after "docker compose".
func composeArgs(file, envFile string, profiles, args []string) []string {
	built := []string{"--env-file", envFile, "-f", file}
	for _, profile := range profiles {
		built = append(built, "--profile", profile)
	}
	return append(built, args...)
}

// runOnceArgs builds the service's image before running it: the image name is
// fixed across runs, so without the build a run replays whatever agent an
// earlier checkout left under that name.
func runOnceArgs(service string, args []string) []string {
	return append([]string{"run", "--rm", "--no-TTY", "--build", service}, args...)
}

func runSplitArgs(service, entrypoint string, args []string) []string {
	run := []string{"run", "--rm", "--no-TTY"}
	if entrypoint != "" {
		run = append(run, "--entrypoint", entrypoint)
	}
	return append(append(run, service), args...)
}
