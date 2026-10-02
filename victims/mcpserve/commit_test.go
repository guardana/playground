package mcpserve_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

type chargeInput struct {
	Amount int64 `json:"amount"`
}

type chargeOutput struct {
	Amount int64 `json:"amount"`
}

// commitProbe serves one committing tool whose run is prepare, and counts the
// changes applied.
func commitProbe(t *testing.T, prepare func(chargeInput) (mcpserve.Commit, error)) (
	*mcp.ClientSession, *mcpserve.Recorder, mcpserve.Config, *int,
) {
	t.Helper()
	config := configIn(t.TempDir())
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	applied := new(int)
	server := mcpserve.NewServer("victim-probe", recorder)
	mcp.AddTool(server, &mcp.Tool{Name: "probe.charge"}, mcpserve.Committing(recorder, "probe.charge",
		func(_ context.Context, in chargeInput) (chargeOutput, mcpserve.Commit, error) {
			commit, err := prepare(in)
			if commit.Apply != nil {
				inner := commit.Apply
				commit.Apply = func() { inner(); *applied++ }
			}
			return chargeOutput(in), commit, err
		}))
	return connect(t, server), recorder, config, applied
}

// The line is on disk before the change is made: Apply reads the journal and
// finds its own call already recorded, effect included.
func TestCommittingWritesTheLineBeforeTheChange(t *testing.T) {
	var seenAtApply []journal.Entry
	var path string
	session, recorder, config, applied := commitProbe(t, func(in chargeInput) (mcpserve.Commit, error) {
		return mcpserve.Commit{
			Detail: "cus_1",
			Effect: journal.Effect{"amount": journal.Integer(in.Amount)},
			Apply: func() {
				entries, err := journal.ReadFile(path)
				if err != nil {
					t.Error(err)
				}
				seenAtApply = entries
			},
		}, nil
	})
	path = config.JournalPath()

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "probe.charge", Arguments: chargeInput{Amount: 5000}})
	if err != nil || result.IsError {
		t.Fatalf("call: %v %+v", err, result)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if *applied != 1 {
		t.Fatalf("applied %d times, want 1", *applied)
	}
	if len(seenAtApply) != 1 || seenAtApply[0].Status != journal.Served {
		t.Fatalf("the journal held %+v when the change was applied, want the served line", seenAtApply)
	}
	entry := onlyEntry(t, path)
	if want := (journal.Effect{"amount": journal.Integer(5000)}); !entry.Effect.Equal(want) {
		t.Errorf("effect is %s, want %s", entry.Effect, want)
	}
}

// Whatever stops the line from being written stops the change, and the one
// line left for the call says it was refused.
func TestCommittingChangesNothingItCouldNotRecord(t *testing.T) {
	tooMany := journal.Effect{}
	for i := range journal.MaxEffectMembers + 1 {
		tooMany[fmt.Sprintf("m%d", i)] = journal.Integer(int64(i))
	}
	apply := func() {}
	cases := map[string]func(chargeInput) (mcpserve.Commit, error){
		"the tool refused": func(chargeInput) (mcpserve.Commit, error) {
			return mcpserve.Commit{Detail: "cus_1", Effect: journal.Effect{"amount": journal.Integer(1)}, Apply: apply},
				errors.New("no such customer")
		},
		"no effect": func(chargeInput) (mcpserve.Commit, error) {
			return mcpserve.Commit{Detail: "cus_1", Apply: apply}, nil
		},
		"no change to apply": func(chargeInput) (mcpserve.Commit, error) {
			return mcpserve.Commit{Detail: "cus_1", Effect: journal.Effect{"amount": journal.Integer(1)}}, nil
		},
		"an effect the journal refuses": func(chargeInput) (mcpserve.Commit, error) {
			return mcpserve.Commit{Detail: "cus_1", Effect: tooMany, Apply: apply}, nil
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			session, recorder, config, applied := commitProbe(t, prepare)
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "probe.charge", Arguments: chargeInput{Amount: 1}})
			if err == nil && !result.IsError {
				t.Fatal("the call succeeded")
			}
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
			if *applied != 0 {
				t.Errorf("applied %d times, want 0", *applied)
			}
			entry := onlyEntry(t, config.JournalPath())
			if entry.Status != journal.Refused || entry.Effect != nil {
				t.Errorf("entry = %+v, want one refused line with no effect", entry)
			}
		})
	}
}

// A journal that cannot be written leaves the state as it was.
func TestCommittingChangesNothingWhenTheJournalIsGone(t *testing.T) {
	session, recorder, _, applied := commitProbe(t, func(in chargeInput) (mcpserve.Commit, error) {
		return mcpserve.Commit{Detail: "cus_1", Effect: journal.Effect{"amount": journal.Integer(in.Amount)}, Apply: func() {}}, nil
	})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "probe.charge", Arguments: chargeInput{Amount: 1}})
	if err == nil && !result.IsError {
		t.Fatal("the call succeeded with no journal")
	}
	if *applied != 0 {
		t.Errorf("applied %d times, want 0", *applied)
	}
}
