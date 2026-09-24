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
//
//	healthprobe [-ca <pem>] <url>        exit 0 on a 2xx answer
//	healthprobe [-ca <pem>] -show <url>  print the status and at most 1 MiB of the body
//
// -ca trusts exactly the certificates in that file, for a service that serves
// https under a CA it made itself.
//
// -show is how the runner reads the enforcer's /healthz and /brand from inside
// its container: the health listener stays on the lab's networks, and the
// runner never needs a route into them.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// timeout bounds one probe. Docker gives the check its own timeout as well;
// this one is shorter so the failure is the probe's answer rather than a killed
// process with nothing to say.
const timeout = 2 * time.Second

// maxShown bounds what -show prints.
const maxShown = 1 << 20

func main() {
	args := os.Args[1:]
	client := http.DefaultClient
	if len(args) >= 2 && args[0] == "-ca" {
		trusting, err := trust(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		client, args = trusting, args[2:]
	}
	var err error
	switch {
	case len(args) == 1:
		err = probe(client, args[0])
	case len(args) == 2 && args[0] == "-show":
		err = show(client, args[1], os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, "usage: healthprobe [-ca <pem>] [-show] <url>")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// trust returns a client whose only roots are the certificates in path.
func trust(path string) (*http.Client, error) {
	pem, err := os.ReadFile(path) // #nosec G304,G703 -- the CA file the health check names.
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s holds no certificate", path)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	return &http.Client{Transport: transport}, nil
}

// show prints the answer's status line and its body, whatever the status: a
// 503 from a plane that takes no material call is an answer worth reading.
func show(client *http.Client, url string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) // #nosec G704 -- see probe.
	if err != nil {
		return err
	}
	response, err := client.Do(request) // #nosec G704 -- see probe.
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxShown+1))
	if err != nil {
		return err
	}
	if len(body) > maxShown {
		return fmt.Errorf("%s answered more than %d bytes", url, maxShown)
	}
	_, err = fmt.Fprintf(out, "status %d\n%s", response.StatusCode, body)
	return err
}

func probe(client *http.Client, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// #nosec G704 -- fetching the URL it was given is the whole of this program,
	// and the only caller is a health check in a compose file.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request) // #nosec G704 -- as above.
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%s answered %d", url, response.StatusCode)
	}
	return nil
}
