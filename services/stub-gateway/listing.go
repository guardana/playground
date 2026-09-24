package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// What the gateway offers, and when it asks the victims what that is.
//
// Every tools/list is answered from the upstreams rather than from what they
// said at startup. A victim whose description changes between listings is one of
// the lies this lab exists to carry — victims/fs re-describes fs.read on its
// second listing and pins the drift with a test of its own — and a registry
// filled in once at boot would answer every listing from the first thing the
// victim said, so the drift would stop here.

// listedTool is one upstream tool as its server describes it now, and the
// upstream a call on it is routed to.
type listedTool struct {
	up   upstream
	tool *mcp.Tool
}

// listAll asks every upstream what it offers.
//
// It refuses two upstreams offering one name, because routing would be a coin
// toss and the trail would name a server the call never reached, and it refuses
// a tool the SDK would panic on rather than register. Both are answers from a
// service the lab did not write, so they are read before anything is registered.
func listAll(ctx context.Context, upstreams []upstream) ([]listedTool, error) {
	bounded, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()

	listed := make([]listedTool, 0, len(upstreams))
	offeredBy := make(map[string]string)
	for _, up := range upstreams {
		for tool, err := range up.session.Tools(bounded, nil) {
			if err != nil {
				return nil, fmt.Errorf("stub-gateway: listing tools of %s: %w", up.name, err)
			}
			if owner, taken := offeredBy[tool.Name]; taken {
				return nil, fmt.Errorf("stub-gateway: %s and %s both offer %s", owner, up.name, tool.Name)
			}
			if err := registrable(tool); err != nil {
				return nil, fmt.Errorf("stub-gateway: %s offers %s: %w", up.name, tool.Name, err)
			}
			offeredBy[tool.Name] = up.name
			listed = append(listed, listedTool{up: up, tool: tool})
		}
	}
	return listed, nil
}

// register makes the server offer exactly what listed says, carrying each
// upstream's annotations and description through unchanged. Adding replaces a
// tool of the same name, so a changed description reaches the client that asked
// for it; a name no upstream offers any more is withdrawn rather than left
// routing calls to a server that no longer says it serves them.
// One round of registrations is applied under the lock, so two listings racing
// each other leave the client a set from one of them rather than a mixture.
func (g *gateway) register(server *mcp.Server, listed []listedTool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	offered := make(map[string]string, len(listed))
	for _, entry := range listed {
		server.AddTool(entry.tool, g.handlerFor(entry.up, entry.tool.Name))
		offered[entry.tool.Name] = entry.up.name
	}
	for name := range g.offered {
		if _, still := offered[name]; !still {
			server.RemoveTools(name)
		}
	}
	g.offered = offered
}

// relist answers tools/list from the upstreams, in front of the handler that
// answers it from the registry.
//
// A listing the gateway cannot refresh fails the call rather than falling back
// to what it registered before: a stale answer read as the current one is the
// same defect in a quieter form.
func (g *gateway) relist(server *mcp.Server, upstreams []upstream) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				listed, err := listAll(ctx, upstreams)
				if err != nil {
					return nil, err
				}
				g.register(server, listed)
			}
			return next(ctx, method, request)
		}
	}
}
