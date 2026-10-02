package mcpserve

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
)

// Commit is what a committing tool hands back before anything has changed:
// the line to journal, and the change to make once that line is on disk.
type Commit struct {
	Detail string
	Effect journal.Effect
	Apply  func()
}

var errNothingCommitted = errors.New("the tool prepared no effect to commit")

// Committing wraps a tool whose served call changes state a scenario grades by
// value. The served line, effect included, is written before Apply runs, so
// the worst a failure leaves is a line for a change that did not happen: a
// scenario expecting none goes red. The other order could leave a charge with
// no line, or under a refusal's.
//
// A run error, a missing effect or a line that could not be written applies
// nothing, and the call is journalled refused, which is then true.
func Committing[In, Out any](recorder *Recorder, tool string,
	run func(context.Context, In) (Out, Commit, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		recorder.commit.Lock()
		defer recorder.commit.Unlock()

		var zero Out
		output, commit, runErr := run(ctx, input)
		if runErr == nil && (len(commit.Effect) == 0 || commit.Apply == nil) {
			runErr = fmt.Errorf("%s: %w", tool, errNothingCommitted)
		}
		if runErr != nil {
			if err := recorder.record(tool, journal.Refused, commit.Detail, nil); err != nil {
				return nil, zero, fmt.Errorf("%s: journal: %w", tool, err)
			}
			markRecorded(ctx)
			return nil, zero, runErr
		}
		if err := recorder.record(tool, journal.Served, commit.Detail, commit.Effect); err != nil {
			// Left unmarked, so the refusal middleware files the call as
			// refused: nothing was applied.
			return nil, zero, fmt.Errorf("%s: journal: %w", tool, err)
		}
		commit.Apply()
		markRecorded(ctx)
		return nil, output, nil
	}
}
