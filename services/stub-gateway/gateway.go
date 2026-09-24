package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/evidence"
)

// The keys the scripted agent stamps on every call, so a decision in the trail
// can be tied to the step of the trajectory that caused it.
const (
	metaStepKey  = "io.guardana.playground/step"
	metaRunIDKey = "io.guardana.playground/run-id"
)

// stubVersion is what this server tells a client it is. A client that logs the
// implementation it connected to should not be able to mistake this for the
// enforcement plane.
const stubVersion = "stub"

// maxStep bounds the step number a caller may stamp on a call. It exists so a
// number from the wire cannot become an index or an unbounded string.
const maxStep = 1 << 20

// upstreamTimeout bounds one exchange with a victim: the handshake that
// connects to it, a listing, or a forwarded call.
//
// The victims sit on the same compose network and answer in milliseconds — the
// slowest hop in the lab is victim-mail delivering to mailpit beside it — so
// thirty seconds is far above anything a working exchange takes. It is not a
// latency budget. It is there because a victim that accepts a call and never
// answers would leave the trail at ACTION_STARTED, which the chain rule reads as
// a request still in flight rather than as a failure: the hang would be graded
// as silence. On expiry the call fails, and ACTION_FAILED is what the contract
// has to say about an action that did not finish.
const upstreamTimeout = 30 * time.Second

// upstream is one victim tool server this gateway sits in front of.
type upstream struct {
	name    string
	session *mcp.ClientSession
}

// gateway replays declared verdicts and records what it did. It decides
// nothing: the verdict for a step is read from the file, and the only thing
// this type computes is whether that verdict forwards.
type gateway struct {
	name     string
	verdicts *verdicts
	trail    *trail
	// callTimeout bounds one forwarded call. It is a field rather than the
	// constant itself so a test can watch the expiry without waiting for it.
	callTimeout time.Duration

	// offered is the tool names registered now, against the upstream each is
	// routed to. It changes on every listing, and listings arrive concurrently.
	mu      sync.Mutex
	offered map[string]string
}

func newGateway(name string, declared *verdicts, records *trail) *gateway {
	return &gateway{name: name, verdicts: declared, trail: records, callTimeout: upstreamTimeout}
}

// newServer offers every upstream tool under its own name, carrying the
// upstream's annotations through unchanged. The lie a victim tells about its
// own tool has to survive this hop, or the lab is testing the gateway's opinion
// of an annotation rather than the annotation.
//
// The listing at startup is what routes a call from an agent that never lists
// anything, and the scripted agent never does. Every tools/list after it asks
// the upstreams again; see listing.go.
func (g *gateway) newServer(ctx context.Context, upstreams []upstream) (*mcp.Server, error) {
	server := mcp.NewServer(&mcp.Implementation{Name: g.name, Version: stubVersion}, nil)
	listed, err := listAll(ctx, upstreams)
	if err != nil {
		return nil, err
	}
	if len(listed) == 0 {
		return nil, errors.New("stub-gateway: no upstream offered a tool")
	}
	g.register(server, listed)
	server.AddReceivingMiddleware(g.relist(server, upstreams))
	return server, nil
}

// registrable reports a tool the SDK would panic on rather than register. An
// upstream is a service the lab did not write, so its answer is refused —
// at startup, or at the listing that first carried it — instead of crashing the
// gateway in the middle of a run.
func registrable(tool *mcp.Tool) error {
	if tool.InputSchema == nil {
		return errors.New("no input schema")
	}
	if schema, ok := tool.InputSchema.(map[string]any); ok && schema["type"] != "object" {
		return fmt.Errorf("input schema type is %v, want object", schema["type"])
	}
	return nil
}

// call is one tools/call as this gateway sees it.
type call struct {
	up     upstream
	tool   string
	params *mcp.CallToolParamsRaw
	// step is the trajectory step as the trail spells it: a decimal string, or
	// empty when the caller stamped no step on the call.
	step   string
	digest string
	trail  *requestTrail
}

func (g *gateway) handlerFor(up upstream, tool string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		number, numbered := stepFromMeta(req.Params.Meta)
		current := &call{
			up:     up,
			tool:   tool,
			params: req.Params,
			step:   stepID(number, numbered),
			digest: digestOf([]byte(up.name), []byte(tool), req.Params.Arguments),
			trail:  g.trail.request(),
		}
		return g.handle(ctx, current, g.verdicts.at(number))
	}
}

// handle records the call, records the declared answer, and then either
// forwards it or does not. The two records are written before anything is
// forwarded: a call that ran without its proposal on disk would be an effect
// with no account of why it happened.
func (g *gateway) handle(ctx context.Context, current *call, declared answer) (*mcp.CallToolResult, error) {
	err := current.trail.append(evidence.Event{
		Kind:     evidence.KindActionProposed,
		Proposed: g.envelope(current),
	})
	if err != nil {
		return nil, err
	}
	err = current.trail.append(evidence.Event{
		Kind:     evidence.KindPolicyDecided,
		Decision: g.decision(current, declared),
	})
	if err != nil {
		return nil, err
	}
	if !declared.forwards() {
		return g.block(current, declared)
	}
	return g.forward(ctx, current)
}

func (g *gateway) block(current *call, declared answer) (*mcp.CallToolResult, error) {
	if err := current.trail.append(evidence.Event{Kind: evidence.KindActionBlocked}); err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: refusal(g.name, declared)}},
	}, nil
}

func (g *gateway) forward(ctx context.Context, current *call) (*mcp.CallToolResult, error) {
	execution := newID("exe")
	startedAt := time.Now().UTC()
	started := &evidence.ActionResult{
		SchemaVersion: wireSchemaVersion,
		RequestID:     current.trail.requestID(),
		ExecutionID:   execution,
		StartedAt:     &startedAt,
	}
	if err := current.trail.append(evidence.Event{
		Kind:        evidence.KindActionStarted,
		ExecutionID: execution,
		Result:      started,
	}); err != nil {
		return nil, err
	}

	bounded, cancel := context.WithTimeout(ctx, g.callTimeout)
	defer cancel()
	result, err := current.up.session.CallTool(bounded, &mcp.CallToolParams{
		Name:      current.tool,
		Arguments: current.params.Arguments,
		Meta:      current.params.Meta,
	})
	if err != nil {
		return nil, errors.Join(err, g.close(current, evidence.KindActionFailed, started))
	}
	// A result carrying IsError is the tool reporting that it did not do what
	// was asked. The action ran; it did not succeed.
	kind := evidence.KindActionCompleted
	if result.IsError {
		kind = evidence.KindActionFailed
	}
	if err := g.close(current, kind, started); err != nil {
		return nil, err
	}
	return result, nil
}

func (g *gateway) close(current *call, kind evidence.Kind, started *evidence.ActionResult) error {
	endedAt := time.Now().UTC()
	finished := *started
	finished.EndedAt = &endedAt
	// The gateway forwards the arguments it was given, so what executed is what
	// was proposed. A producer that changed them would say so here.
	finished.ExecutedActionDigest = current.digest
	return current.trail.append(evidence.Event{
		Kind:        kind,
		ExecutionID: started.ExecutionID,
		Result:      &finished,
	})
}

func refusal(name string, declared answer) string {
	codes := strings.Join(declared.reasonCodes, ", ")
	if codes == "" {
		codes = "no reason code declared"
	}
	return fmt.Sprintf("%s did not forward this call: %s (%s)", name, declared.verdict, codes)
}
