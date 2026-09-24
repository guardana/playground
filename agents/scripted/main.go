// Command scripted-agent replays one trajectory as real MCP calls through the
// gateway.
//
//	scripted-agent -trajectory <path> -gateway <url> -run-id <id> -out <path>
//	scripted-agent -probe <host:port>
//
// No model decides anything here. The trajectory is the whole of what happens,
// so two runs of one file make the same calls and a red run is about the
// systems under test rather than about what a model felt like doing.
//
// The agent writes its own log to -out. **Nothing asserts on that file.** A
// scenario is graded from the evidence trail the gateway wrote and the journals
// the victims kept; an agent's account of its own work is narrative, and this
// repository does not grade narrative. The log exists so a person reading a red
// run can see which step the agent thought it was on.
//
// -probe dials one address and exits zero only if a TCP connection was made. It
// exists so the runner can prove, from inside agent-net, that a victim is not
// reachable except through the gateway. A name that does not resolve, a refused
// connection and a timeout are one answer: not reachable.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/guardana/playground/internal/labspec"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	agentName    = "scripted-agent"
	agentVersion = "1"
	// defaultTimeout bounds a whole replay. A trajectory that hangs is a red
	// run that never reports, which is worse than a red run.
	defaultTimeout = 2 * time.Minute
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", agentName, err)
		os.Exit(1)
	}
}

type options struct {
	trajectory string
	gateway    string
	runID      string
	out        string
	probe      string
	timeout    time.Duration
}

func run(ctx context.Context, args []string, out io.Writer) error {
	settings, err := parse(args, out)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return nil
	case err != nil:
		return err
	}
	if settings.probe != "" {
		return runProbe(ctx, settings.probe, out)
	}
	return runTrajectory(ctx, settings)
}

func parse(args []string, out io.Writer) (options, error) {
	var settings options
	set := flag.NewFlagSet(agentName, flag.ContinueOnError)
	set.SetOutput(out)
	set.StringVar(&settings.trajectory, "trajectory", "", "path to the trajectory to replay")
	set.StringVar(&settings.gateway, "gateway", "", "the gateway's MCP endpoint, for example http://stub-gateway:8080/mcp")
	set.StringVar(&settings.runID, "run-id", "", "the identifier of this run, minted by the runner")
	set.StringVar(&settings.out, "out", "", "where to write the agent's own log, as JSON lines")
	set.StringVar(&settings.probe, "probe", "", "dial host:port, exit zero only if a TCP connection was made")
	set.DurationVar(&settings.timeout, "timeout", defaultTimeout, "how long the whole replay may take")
	if err := set.Parse(args); err != nil {
		return options{}, err
	}
	if settings.probe != "" {
		// A probe answers one question and needs nothing else.
		return settings, nil
	}
	for _, required := range []struct{ name, value string }{
		{"-trajectory", settings.trajectory},
		{"-gateway", settings.gateway},
		{"-run-id", settings.runID},
		{"-out", settings.out},
	} {
		if required.value == "" {
			return options{}, fmt.Errorf("%s is required", required.name)
		}
	}
	return settings, nil
}

func runTrajectory(ctx context.Context, settings options) error {
	trajectory, err := labspec.LoadTrajectory(settings.trajectory)
	if err != nil {
		return err
	}
	log, err := createLog(settings.out)
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()

	ctx, cancel := context.WithTimeout(ctx, settings.timeout)
	defer cancel()

	session, err := connect(ctx, settings.gateway)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", settings.gateway, err)
	}
	defer func() { _ = session.Close() }()

	return replay(ctx, session, trajectory, settings.runID, log)
}

// connect opens one MCP session to the gateway.
func connect(ctx context.Context, endpoint string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: agentName, Version: agentVersion}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint,
		// A replay needs request and response and nothing else. The standalone
		// stream is a GET the client leaves open for the lifetime of the
		// session, and a server waits on it at shutdown, so leaving it on turns
		// a finished run into a hung one.
		DisableStandaloneSSE: true,
	}, nil)
}

func createLog(path string) (*os.File, error) {
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return nil, err
		}
	}
	// #nosec G304 -- the path is the log the caller asked for.
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
}
