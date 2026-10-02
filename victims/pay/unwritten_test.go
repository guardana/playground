package main

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A served call whose line cannot be written must leave the ledger as it was:
// a scenario reads money movements from the journal alone, so a change with no
// line is one no run can see. The journal is closed after the server is up, so
// each call prepares its change and then fails to record it.
func TestACallItCannotJournalChangesNothing(t *testing.T) {
	tests := map[string]map[string]any{
		"pay.charge": {"customer_id": "cus_9001", "amount": 100, "currency": "EUR"},
		"pay.refund": {"charge_id": "ch_0001", "amount": 100},
		"pay.payout": {"destination": "acct_lab_9001", "amount": 100, "currency": "EUR"},
	}
	for tool, arguments := range tests {
		t.Run(tool, func(t *testing.T) {
			lab := start(t)
			before := snapshot(lab.book)
			if err := lab.recorder.Close(); err != nil {
				t.Fatal(err)
			}
			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
			if err == nil && !result.IsError {
				t.Fatalf("served with no journal: %s", text(result))
			}
			if after := snapshot(lab.book); !after.equal(before) {
				t.Errorf("the ledger changed: %+v, was %+v", after, before)
			}
			if entries := lab.entries(t); len(entries) != 0 {
				t.Errorf("a closed journal holds %+v", entries)
			}
		})
	}
}
