// The health probe answers one question from inside a container: is the
// service on this address ready?
//
// It exists because the lab's services run on a distroless base, which has no
// shell and no curl, and a Docker health check runs inside the container it
// checks. Without it `depends_on` could only wait for a container to have been
// started, which is how a runner ends up asserting against a service that has
// not finished starting.
//
// It is copied into every image the lab builds and is never part of a service.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// timeout bounds one probe. Docker gives the check its own timeout as well;
// this one is shorter so the failure is the probe's answer rather than a killed
// process with nothing to say.
const timeout = 2 * time.Second

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: healthprobe <url>")
		os.Exit(2)
	}
	if err := probe(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func probe(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// #nosec G704 -- fetching the URL it was given is the whole of this program,
	// and the only caller is a health check in a compose file.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request) // #nosec G704 -- as above.
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%s answered %d", url, response.StatusCode)
	}
	return nil
}
