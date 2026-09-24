package mcpserve

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
)

// Recorder writes one server's journal.
//
// Its methods take no context on purpose. The write is a local append that has
// to complete even when the caller's context is already cancelled: a served
// call missing from the journal reads as a call that never arrived, and that is
// the direction that hides a defect.
type Recorder struct {
	writer *journal.Writer
	runID  string
}

// NewRecorder stamps runID on every entry written through it.
func NewRecorder(writer *journal.Writer, runID string) *Recorder {
	return &Recorder{writer: writer, runID: runID}
}

// Served records a call the server answered. Detail is for a person reading a
// failed run: the path, the recipient, the statement.
func (r *Recorder) Served(tool, detail string) error {
	return r.record(tool, journal.Served, detail)
}

// Refused records a call the server received and did not run.
func (r *Recorder) Refused(tool, detail string) error {
	return r.record(tool, journal.Refused, detail)
}

func (r *Recorder) record(tool string, status journal.Status, detail string) error {
	return r.writer.Record(journal.Entry{
		OccurredAt: time.Now().UTC(),
		Tool:       truncate(tool),
		RunID:      r.runID,
		Status:     status,
		Detail:     truncate(detail),
	})
}

// Close flushes the journal. The runner reads the file after the container it
// was written in has stopped.
func (r *Recorder) Close() error { return r.writer.Close() }

// Journalled wraps a tool implementation so that no path through it can answer
// without being recorded. run returns what to answer with, the detail to
// journal, and whether the call was refused; an error is a refusal, because the
// server received the call and did not run the tool.
//
// Every handler in every victim goes through here. A tool that answers without
// journalling is a hole in every effect assertion in the lab.
func Journalled[In, Out any](recorder *Recorder, tool string,
	run func(context.Context, In) (Out, string, error),
) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		output, detail, runErr := run(ctx, input)

		status := journal.Served
		if runErr != nil {
			status = journal.Refused
		}
		if err := recorder.record(tool, status, detail); err != nil {
			// The call happened and the record of it did not. Failing the call
			// is the safe direction: the scenario reads effects from the
			// journal, so answering here would let a run pass on a file that
			// does not describe it.
			var zero Out
			return nil, zero, fmt.Errorf("%s: journal: %w", tool, err)
		}
		markRecorded(ctx)
		return nil, output, runErr
	}
}

// Hint returns a pointer to v. The SDK models the annotations that have a
// non-false default as pointers, so that unset and false are different answers,
// and a victim that lies has to say false out loud.
func Hint(v bool) *bool { return &v }

// detailLimit is what one field of a journal line may carry. Both fields a
// caller controls are bounded by it, and the two together stay two orders of
// magnitude below journal.MaxLineBytes, which is where the reader gives up on
// the whole file. A few kilobytes is more than a person reading a failed run
// needs from a path, a statement or a recipient list.
const detailLimit = 4 << 10

// truncate bounds a field and says so in the field. A detail that stops
// mid-sentence with no marker reads as the thing that happened, and a statement
// the agent chose is the easiest thing in the lab to make a megabyte long.
func truncate(text string) string {
	if len(text) <= detailLimit {
		return text
	}
	// Cut on a byte boundary, so drop whatever rune the cut landed inside
	// rather than writing a replacement character into the record.
	return strings.ToValidUTF8(text[:detailLimit], "") +
		fmt.Sprintf(" [truncated, %d bytes]", len(text))
}
