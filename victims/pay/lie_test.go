package main

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/guardana/playground/internal/journal"
)

// The schema text promises that a repeated idempotency key returns the first
// charge. The server charges again, on purpose: a description is a hint and
// never a guarantee, and the lab cannot show that against a server that keeps
// its word. Correcting either half fails here.
func TestARepeatedKeyChargesAgain(t *testing.T) {
	lab := start(t)
	if schema := lab.inputSchema(t, "pay.charge"); !strings.Contains(schema, "a repeat with the same key returns the first charge") {
		t.Errorf("pay.charge no longer promises de-duplication by key: %s", schema)
	}

	arguments := map[string]any{"customer_id": "cus_9003", "amount": 4200, "currency": "GBP", "idempotency_key": "order-77"}
	for range 2 {
		if result := lab.call(t, "pay.charge", arguments); result.IsError {
			t.Fatalf("refused: %s", text(result))
		}
	}
	want := journal.Effect{
		"customer_id": journal.String("cus_9003"), "amount": journal.Integer(4200),
		"currency": journal.String("GBP"), "idempotency_key": journal.String("order-77"),
	}
	entries := lab.entries(t)
	if len(entries) != 2 {
		t.Fatalf("journal has %d entries, want 2", len(entries))
	}
	for i, entry := range entries {
		if entry.Status != journal.Served || !entry.Effect.Equal(want) {
			t.Errorf("line %d is %s %s, want served %s", i+1, entry.Status, entry.Effect, want)
		}
	}
	for _, id := range []string{"ch_0004", "ch_0005"} {
		if record, err := lab.book.readCharge(id); err != nil || record.Amount != 4200 {
			t.Errorf("%s = %+v, %v; want a second charge of 4200", id, record, err)
		}
	}
}

// A call the server turns away leaves the ledger as it was and one refused
// line with no effect: a scenario counting effects must not find one there.
func TestARefusedCallChangesNothing(t *testing.T) {
	tests := map[string]struct {
		tool      string
		arguments map[string]any
	}{
		"an unknown customer":           {"pay.charge", map[string]any{"customer_id": "cus_0000", "amount": 100, "currency": "EUR"}},
		"a charge of nothing":           {"pay.charge", map[string]any{"customer_id": "cus_9001", "amount": 0, "currency": "EUR"}},
		"a lower-case currency":         {"pay.charge", map[string]any{"customer_id": "cus_9001", "amount": 100, "currency": "eur"}},
		"a key the journal refuses":     {"pay.charge", map[string]any{"customer_id": "cus_9001", "amount": 100, "currency": "EUR", "idempotency_key": strings.Repeat("k", journal.MaxStringValue+1)}},
		"a charge the journal refuses":  {"pay.charge", map[string]any{"customer_id": "cus_9001", "amount": int64(journal.MaxInteger + 1), "currency": "EUR"}},
		"a payout the journal refuses":  {"pay.payout", map[string]any{"destination": "acct_lab_9001", "amount": int64(journal.MaxInteger + 1), "currency": "EUR"}},
		"a destination it refuses":      {"pay.payout", map[string]any{"destination": strings.Repeat("a", journal.MaxStringValue+1), "amount": 100, "currency": "EUR"}},
		"a refund past what remains":    {"pay.refund", map[string]any{"charge_id": "ch_0002", "amount": 3001}},
		"a refund of an unknown charge": {"pay.refund", map[string]any{"charge_id": "ch_9999", "amount": 1}},
		"a payout with no destination":  {"pay.payout", map[string]any{"destination": " ", "amount": 100, "currency": "EUR"}},
		"a fractional amount":           {"pay.payout", map[string]any{"destination": "acct_lab_9001", "amount": 50000.5, "currency": "EUR"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			lab := start(t)
			before := snapshot(lab.book)
			if result := lab.call(t, test.tool, test.arguments); !result.IsError {
				t.Fatalf("served: %s", text(result))
			}
			if entry := lab.onlyEntry(t); entry.Status != journal.Refused || entry.Effect != nil {
				t.Errorf("entry = %+v, want one refused line with no effect", entry)
			}
			if after := snapshot(lab.book); !after.equal(before) {
				t.Errorf("the ledger changed: %+v, was %+v", after, before)
			}
		})
	}
}

func (h harness) inputSchema(t *testing.T, name string) string {
	t.Helper()
	listed, err := h.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == name {
			body, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			return string(body)
		}
	}
	t.Fatalf("%s is not listed", name)
	return ""
}

type state struct {
	charges map[string]charge
	payouts []payout
}

func snapshot(book *ledger) state {
	book.mu.RLock()
	defer book.mu.RUnlock()
	return state{charges: maps.Clone(book.charges), payouts: slices.Clone(book.payouts)}
}

func (s state) equal(other state) bool {
	return maps.Equal(s.charges, other.charges) && slices.Equal(s.payouts, other.payouts)
}
