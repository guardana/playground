package main

import (
	"fmt"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
)

const callers = 16

// Each of these refunds would empty the charge on its own. They race, and
// exactly one may be served: two served from one prepared balance would pay
// the charge back twice.
func TestConcurrentFullRefundsServeOnce(t *testing.T) {
	lab := start(t)
	results := lab.callAll(t, func(int) (string, map[string]any) {
		return "pay.refund", map[string]any{"charge_id": "ch_0001", "amount": 12000}
	})
	if served := lab.servedLines(t, "pay.refund"); served != 1 || answered(results) != 1 {
		t.Errorf("%d served lines and %d answers, want 1 of each", served, answered(results))
	}
	if record, err := lab.book.readCharge("ch_0001"); err != nil || record.Refunded != 12000 {
		t.Errorf("ch_0001 = %+v, %v; want 12000 refunded once", record, err)
	}
}

// Concurrent charges, some of them refused, each take their own id: every
// served line is a charge in the ledger and no charge lacks one.
func TestConcurrentChargesEachLandOnce(t *testing.T) {
	lab := start(t)
	results := lab.callAll(t, func(i int) (string, map[string]any) {
		customer := fmt.Sprintf("cus_900%d", 1+i%3)
		if i%4 == 0 {
			customer = "cus_0000"
		}
		return "pay.charge", map[string]any{"customer_id": customer, "amount": 100 + i, "currency": "EUR"}
	})
	served := lab.servedLines(t, "pay.charge")
	if served != answered(results) || served != callers-callers/4 {
		t.Errorf("%d served lines and %d answers, want %d", served, answered(results), callers-callers/4)
	}
	if got := len(snapshot(lab.book).charges); got != 3+served {
		t.Errorf("the ledger holds %d charges, want 3 seeded and %d served", got, served)
	}
}

func (h harness) callAll(t *testing.T, call func(int) (string, map[string]any)) []*mcp.CallToolResult {
	t.Helper()
	results := make([]*mcp.CallToolResult, callers)
	errs := make([]error, callers)
	var group sync.WaitGroup
	for i := range callers {
		tool, arguments := call(i)
		group.Go(func() {
			results[i], errs[i] = h.session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
		})
	}
	group.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	return results
}

func answered(results []*mcp.CallToolResult) int {
	count := 0
	for _, result := range results {
		if !result.IsError {
			count++
		}
	}
	return count
}

// servedLines counts the served lines for tool, and fails the test unless
// every call made has a line of its own.
func (h harness) servedLines(t *testing.T, tool string) int {
	t.Helper()
	entries := h.entries(t)
	if len(entries) != callers {
		t.Fatalf("journal has %d lines for %d calls", len(entries), callers)
	}
	count := 0
	for _, entry := range entries {
		if entry.Tool == tool && entry.Status == journal.Served {
			count++
		}
	}
	return count
}
