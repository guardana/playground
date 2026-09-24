package mcpserve

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The lab pins one version for every victim: they are released together, and a
// version that varies invites a scenario to depend on it.
const serverVersion = "1"

// NewServer builds the MCP server a victim serves. Every victim is built here
// so that the refusal middleware is not something a server can be written
// without.
func NewServer(name string, recorder *Recorder) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: name, Version: serverVersion}, nil)
	server.AddReceivingMiddleware(refusals(recorder))
	return server
}

// refusals journals a tools/call that no handler recorded: an unknown tool, or
// arguments the schema rejects. The SDK turns both away before any handler
// runs, so nothing inside a victim can see them, and a call missing from the
// journal reads as a call that never arrived — which is what a denial by the
// gateway looks like. The two have to be told apart.
func refusals(recorder *Recorder) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, request)
			}
			marked := new(atomic.Bool)
			result, err := next(context.WithValue(ctx, recordedKey{}, marked), method, request)
			if marked.Load() {
				return result, err
			}
			call, ok := request.(*mcp.CallToolRequest)
			if !ok {
				return result, err
			}
			// A name the server does not know still names what was asked for.
			_ = recorder.Refused(call.Params.Name, refusalReason(result, err))
			return result, err
		}
	}
}

// recordedKey carries the flag a handler sets once it has journalled the call
// it is serving. It is per call, so two calls in flight cannot answer for each
// other.
type recordedKey struct{}

func markRecorded(ctx context.Context) {
	if marked, ok := ctx.Value(recordedKey{}).(*atomic.Bool); ok {
		marked.Store(true)
	}
}

// refusalReason says why, at whatever length the SDK or the client chose. What
// a journal line may carry is bounded in one place, on the way in to Record.
func refusalReason(result mcp.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	call, ok := result.(*mcp.CallToolResult)
	if !ok {
		return "refused before the tool ran"
	}
	var reason strings.Builder
	for _, content := range call.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			reason.WriteString(text.Text)
		}
	}
	return reason.String()
}
