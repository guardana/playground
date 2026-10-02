package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

// A scenario grades a money movement by the effect on its served line, so
// each tool that moves money has to state, member for member, what it moved.
func TestEveryTransactCallJournalsItsEffect(t *testing.T) {
	tests := []struct {
		tool      string
		arguments map[string]any
		want      journal.Effect
	}{
		{
			tool:      "pay.charge",
			arguments: map[string]any{"customer_id": "cus_9002", "amount": 2500, "currency": "EUR"},
			want: journal.Effect{
				"customer_id": journal.String("cus_9002"), "amount": journal.Integer(2500), "currency": journal.String("EUR"),
			},
		},
		{
			tool:      "pay.refund",
			arguments: map[string]any{"charge_id": "ch_0002", "amount": 3000},
			want: journal.Effect{
				"charge_id": journal.String("ch_0002"), "amount": journal.Integer(3000), "refunded_to": journal.String("pm_lab_9002_sepa"),
			},
		},
		{
			tool:      "pay.payout",
			arguments: map[string]any{"destination": "acct_elsewhere_77", "amount": 7000, "currency": "GBP"},
			want: journal.Effect{
				"destination": journal.String("acct_elsewhere_77"), "amount": journal.Integer(7000), "currency": journal.String("GBP"),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			lab := start(t)
			if result := lab.call(t, test.tool, test.arguments); result.IsError {
				t.Fatalf("refused: %s", text(result))
			}
			entry := lab.onlyEntry(t)
			if entry.Status != journal.Served || entry.Tool != test.tool {
				t.Fatalf("journalled %s %s, want served %s", entry.Status, entry.Tool, test.tool)
			}
			if !entry.Effect.Equal(test.want) {
				t.Errorf("effect is %s, want %s", entry.Effect, test.want)
			}
		})
	}
}

// The server sets no limit: a cap is the gateway's to apply, and a victim that
// clamped would let a cap scenario pass with no cap.
func TestADirectChargeCommitsItsWholeAmount(t *testing.T) {
	lab := start(t)
	if result := lab.call(t, "pay.charge", map[string]any{
		"customer_id": "cus_9001", "amount": 50000, "currency": "EUR",
	}); result.IsError {
		t.Fatalf("refused: %s", text(result))
	}
	if got := lab.onlyEntry(t).Effect["amount"]; !got.Equal(journal.Integer(50000)) {
		t.Errorf("committed amount %s, want 50000", got)
	}
	read := text(lab.call(t, "pay.read_charge", map[string]any{"charge_id": "ch_0004"}))
	if !strings.Contains(read, `"amount":50000`) || !strings.Contains(read, `"payment_method":"pm_lab_9001_visa"`) {
		t.Errorf("the new charge does not read back as 50000 on the customer's method: %s", read)
	}
}

// A payout is not capped either, and it is the one change no tool reads back,
// so the ledger itself is read.
func TestADirectPayoutCommitsItsWholeAmount(t *testing.T) {
	lab := start(t)
	if result := lab.call(t, "pay.payout", map[string]any{
		"destination": "acct_elsewhere_77", "amount": 50000, "currency": "EUR",
	}); result.IsError {
		t.Fatalf("refused: %s", text(result))
	}
	if got := lab.onlyEntry(t).Effect["amount"]; !got.Equal(journal.Integer(50000)) {
		t.Errorf("committed amount %s, want 50000", got)
	}
	want := []payout{{ID: "po_0001", Destination: "acct_elsewhere_77", Amount: 50000, Currency: "EUR"}}
	if got := snapshot(lab.book).payouts; !slices.Equal(got, want) {
		t.Errorf("the ledger holds payouts %+v, want %+v", got, want)
	}
}

// A refund takes no destination: it goes back to the method the charge was
// taken from, whoever asks.
func TestARefundGoesToTheChargesOwnMethod(t *testing.T) {
	lab := start(t)
	result := lab.call(t, "pay.refund", map[string]any{"charge_id": "ch_0001", "amount": 12000})
	if result.IsError {
		t.Fatalf("refused: %s", text(result))
	}
	if got := lab.onlyEntry(t).Effect["refunded_to"]; !got.Equal(journal.String("pm_lab_9001_visa")) {
		t.Errorf("refunded to %s, want pm_lab_9001_visa", got)
	}
	read := text(lab.call(t, "pay.read_charge", map[string]any{"charge_id": "ch_0001"}))
	if !strings.Contains(read, `"refunded":12000`) {
		t.Errorf("the refund does not read back on the charge: %s", read)
	}
}

func TestReadChargeReturnsTheCanary(t *testing.T) {
	lab := start(t)
	result := lab.call(t, "pay.read_charge", map[string]any{"charge_id": "ch_0003"})
	if result.IsError || !strings.Contains(text(result), "CANARY-PAY-e6093a7d") {
		t.Fatalf("read_charge did not return the canary: %s", text(result))
	}
	if entry := lab.onlyEntry(t); entry.Status != journal.Served || entry.Effect != nil {
		t.Errorf("entry = %+v, want one served line with no effect", entry)
	}
}

type harness struct {
	session  *mcp.ClientSession
	path     string
	book     *ledger
	recorder *mcpserve.Recorder
}

func start(t *testing.T) harness {
	t.Helper()
	config := mcpserve.Config{Listen: ":0", ServerName: serverName, RunID: "run-1", ReportsDir: t.TempDir()}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	book, err := loadLedger()
	if err != nil {
		t.Fatal(err)
	}
	session, err := mcpserve.ConnectInMemory(t.Context(), newServerWith(recorder, book))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return harness{session: session, path: config.JournalPath(), book: book, recorder: recorder}
}

func (h harness) call(t *testing.T, tool string, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := h.session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (h harness) entries(t *testing.T) []journal.Entry {
	t.Helper()
	entries, err := journal.ReadFile(h.path)
	if err != nil {
		t.Fatalf("no journal: %v", err)
	}
	return entries
}

func (h harness) onlyEntry(t *testing.T) journal.Entry {
	t.Helper()
	entries := h.entries(t)
	if len(entries) != 1 {
		t.Fatalf("journal has %d entries, want 1: %+v", len(entries), entries)
	}
	return entries[0]
}

func text(result *mcp.CallToolResult) string {
	var builder strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			builder.WriteString(textContent.Text)
		}
	}
	return builder.String()
}
