package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// attackerWeb is the only host this server will fetch from. The lab's
	// network has no route out, so this is a statement about which service is
	// being read rather than a security boundary.
	attackerWeb = "attacker-web"
	fetchLimit  = 10 * time.Second
	// A body larger than this is truncated rather than held in memory. A
	// payload that needs a megabyte to carry an instruction is not the payload
	// this lab is about.
	bodyLimit = 1 << 20
	// maxRedirects is the cap net/http keeps when a client sets no redirect
	// check of its own; one that sets a check has to keep it itself.
	maxRedirects = 10
)

var (
	errHostNotAllowed = errors.New("host is not allowed")
	errNotFetchable   = errors.New("not an http url")
)

// document is what a fetch returns. The body is handed back as it arrived,
// injected instructions included, because that is the payload a toxic flow
// carries.
type document struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// fetcher holds the client and the one host it will talk to.
type fetcher struct {
	client *http.Client
	host   string
}

func (f fetcher) fetch(ctx context.Context, given string) (document, error) {
	target, err := url.Parse(given)
	if err != nil {
		return document{}, err
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return document{}, fmt.Errorf("%w: %s", errNotFetchable, given)
	}
	if target.Host != f.host {
		return document{}, fmt.Errorf("%w: %s", errHostNotAllowed, target.Host)
	}

	ctx, cancel := context.WithTimeout(ctx, fetchLimit)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return document{}, err
	}
	// A redirect is a new request to another host as surely as a URL naming
	// it, so it is held to the same one host.
	client := *f.client
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		switch {
		case next.URL.Host != f.host:
			return fmt.Errorf("%w: %s", errHostNotAllowed, next.URL.Host)
		case len(via) >= maxRedirects:
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return document{}, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, bodyLimit))
	if err != nil {
		return document{}, err
	}
	return document{URL: target.String(), Status: response.StatusCode, Body: string(body)}, nil
}
