package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

// The reason code control's kernel would record for each way an ask can come
// back. controlReads follows control's AuthZEN client at the pinned commit,
// adapters/authzen/{client,answer}.go, rule for rule; nothing here is shared
// with the double, so the double cannot agree with itself.
const (
	codeAllow      = "PDP_ALLOW"
	codeDeny       = "PDP_DENY"
	codeObligation = "OBLIGATION_NOT_UNDERSTOOD"
	codeTimeout    = "PDP_TIMEOUT"
	codeUnavail    = "PDP_UNAVAILABLE"
	codeRefused    = "PDP_ANSWER_REFUSED"
)

// controlReads classifies one exchange the way control's client does: resp and
// err are what http.Client.Do returned under ctx, sent is the X-Request-ID it
// sent. The lab configures no informational context members, so an allowing
// answer may carry obligations in its context and nothing else.
func controlReads(ctx context.Context, resp *http.Response, err error, sent string) string {
	if err != nil {
		return failedAsk(ctx)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return codeUnavail
	}
	ids := resp.Header.Values("X-Request-ID")
	if !declaresJSON(resp.Header.Values("Content-Type")) || len(ids) != 1 || ids[0] != sent {
		return codeRefused
	}
	text, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
	switch {
	case err != nil || ctx.Err() != nil:
		return failedAsk(ctx)
	case len(text) > 64<<10:
		return codeRefused
	}
	return readDecision(text)
}

func failedAsk(ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return codeTimeout
	}
	return codeUnavail
}

func declaresJSON(values []string) bool {
	if len(values) != 1 {
		return false
	}
	media, params, err := mime.ParseMediaType(values[0])
	if err != nil || media != "application/json" {
		return false
	}
	for name, value := range params {
		if name != "charset" || !strings.EqualFold(value, "utf-8") {
			return false
		}
	}
	return true
}

func readDecision(body []byte) string {
	top, err := strictObject(body)
	if err != nil {
		return codeRefused
	}
	switch string(top["decision"]) {
	case "false":
		return codeDeny
	case "true":
		return readAllowed(top)
	}
	return codeRefused
}

func readAllowed(top map[string]json.RawMessage) string {
	raw, ok := top["context"]
	if !ok {
		if len(top) != 1 {
			return codeRefused
		}
		return codeAllow
	}
	inner, err := strictObject(raw)
	if err != nil {
		return codeRefused
	}
	if obligations, ok := inner["obligations"]; ok {
		var items []json.RawMessage
		if json.Unmarshal(obligations, &items) != nil || items == nil {
			return codeRefused
		}
		if len(items) > 0 {
			return codeObligation
		}
	}
	if len(top) != 2 {
		return codeRefused
	}
	for name := range inner {
		if name != "obligations" {
			return codeRefused
		}
	}
	return codeAllow
}

// strictObject reads one JSON object with every member once and nothing after.
func strictObject(raw []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, _ := key.(string)
		if _, twice := out[name]; twice {
			return nil, errors.New("a member twice")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, errors.New("the object does not end")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one document")
	}
	return out, nil
}
