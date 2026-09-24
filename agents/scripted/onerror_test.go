package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/labspec"
)

// Two independent calls, the first of which the scenario expects the gateway
// to answer with an error.
func errorThenRead(onError string) labspec.Trajectory {
	trajectory := twoSteps()
	trajectory.Steps[0].OnError = onError
	trajectory.Steps[1] = labspec.Step{Call: labspec.Call{Server: "victim-crm", Tool: "crm.read_customer"}}
	return trajectory
}

func wireError() error {
	return fmt.Errorf("calling %q: %w", "tools/call", &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "context deadline exceeded"})
}

func TestAStepAnsweredWithAnErrorItAllowsIsNotTheEndOfTheReplay(t *testing.T) {
	gateway := &fakeGateway{answers: []answer{{err: wireError()}, {text: []string{"customer"}}}}
	var log bytes.Buffer
	if err := replay(context.Background(), gateway, errorThenRead(labspec.OnErrorContinue), "run-7", testNamespace, &log, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(gateway.sent) != 2 {
		t.Errorf("sent %d calls, want the second step after the answered error", len(gateway.sent))
	}
	if !strings.Contains(log.String(), `"outcome":"error"`) {
		t.Errorf("the answered error was not logged as one:\n%s", log.String())
	}
}

func TestAnErrorTheStepDoesNotAllowStillEndsTheReplay(t *testing.T) {
	for name, tc := range map[string]struct {
		onError string
		err     error
	}{
		"a wire error on a step that stops": {"", wireError()},
		"a plain error on a step that goes": {labspec.OnErrorContinue, errors.New("connection refused")},
	} {
		t.Run(name, func(t *testing.T) {
			gateway := &fakeGateway{answers: []answer{{err: tc.err}, {text: []string{"customer"}}}}
			err := replay(context.Background(), gateway, errorThenRead(tc.onError), "run-7", testNamespace, &bytes.Buffer{}, nil)
			if err == nil || len(gateway.sent) != 1 {
				t.Errorf("replay went on after %v: err %v, %d calls", tc.err, err, len(gateway.sent))
			}
		})
	}
}

// The step that errored produced nothing, so a flow reading it is not sent.
func TestAStepReadingAnAnsweredErrorIsRefused(t *testing.T) {
	trajectory := twoSteps()
	trajectory.Steps[0].OnError = labspec.OnErrorContinue
	gateway := &fakeGateway{answers: []answer{{err: wireError()}, {text: []string{"queued"}}}}
	err := replay(context.Background(), gateway, trajectory, "run-7", testNamespace, &bytes.Buffer{}, nil)
	if err == nil || len(gateway.sent) != 1 {
		t.Errorf("a step reading an errored step's output was sent: err %v, %d calls", err, len(gateway.sent))
	}
}

// sdkGateway serves the two tools of errorThenRead over streamable HTTP for
// real. fs.read answers with failList when it is set; served counts the calls
// that reached a handler, and broken, once set, answers every request with a
// 502 instead of JSON-RPC.
func sdkGateway(t *testing.T, failList error) (endpoint *httptest.Server, served, broken *atomic.Int32) {
	t.Helper()
	served, broken = &atomic.Int32{}, &atomic.Int32{}
	server := mcp.NewServer(&mcp.Implementation{Name: "gateway", Version: "1"}, nil)
	for _, tool := range []string{"fs.read", "crm.read_customer"} {
		server.AddTool(&mcp.Tool{Name: tool, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				served.Add(1)
				if request.Params.Name == "fs.read" && failList != nil {
					return nil, failList
				}
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "customer"}}}, nil
			})
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	endpoint = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() != 0 {
			http.Error(w, "upstream gone", http.StatusBadGateway)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(endpoint.Close)
	return endpoint, served, broken
}

func TestAProtocolErrorTheGatewaySentLetsTheStepGoOn(t *testing.T) {
	endpoint, served, _ := sdkGateway(t, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "context deadline exceeded"})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := connect(ctx, endpoint.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = session.Close() }()
	if err := replay(ctx, session, errorThenRead(labspec.OnErrorContinue), "run-7", testNamespace, &bytes.Buffer{}, nil); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if served.Load() != 2 {
		t.Errorf("the gateway served %d calls, want the second step after its error", served.Load())
	}
}

// A call the SDK could not deliver comes back as a JSON-RPC error too, but no
// gateway sent it: the step never reached one, so the replay ends there.
func TestACallThatNeverReachedTheGatewayEndsTheReplay(t *testing.T) {
	for name, cut := range map[string]func(*httptest.Server, *atomic.Int32){
		"the gateway gone":            func(endpoint *httptest.Server, _ *atomic.Int32) { endpoint.Close() },
		"an HTTP status, no JSON-RPC": func(_ *httptest.Server, broken *atomic.Int32) { broken.Store(1) },
	} {
		t.Run(name, func(t *testing.T) {
			endpoint, served, broken := sdkGateway(t, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			session, err := connect(ctx, endpoint.URL)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			cut(endpoint, broken)
			var log bytes.Buffer
			err = replay(ctx, session, errorThenRead(labspec.OnErrorContinue), "run-7", testNamespace, &log, nil)
			if err == nil || strings.Count(log.String(), "\n") != 1 || served.Load() != 0 {
				t.Errorf("replay went on past an undelivered call: err %v, %d served, log:\n%s", err, served.Load(), log.String())
			}
		})
	}
}
