package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

func TestToolsAnswerAndAreJournalled(t *testing.T) {
	tests := []struct {
		name       string
		tool       string
		arguments  map[string]any
		wantStatus journal.Status
		wantText   string
	}{
		{
			name:       "a customer of the caller's tenant is read",
			tool:       "crm.read_customer",
			arguments:  map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_4417"},
			wantStatus: journal.Served,
			wantText:   "Northwind Trading",
		},
		{
			name:       "a customer of another tenant is refused",
			tool:       "crm.read_customer",
			arguments:  map[string]any{"tenant_id": "tenant_b", "customer_id": "cus_4417"},
			wantStatus: journal.Refused,
			wantText:   "tenant_b",
		},
		{
			name:       "an unknown customer is refused",
			tool:       "crm.read_customer",
			arguments:  map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_0000"},
			wantStatus: journal.Refused,
			wantText:   "cus_0000",
		},
		{
			name:       "a note is written",
			tool:       "crm.update_note",
			arguments:  map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_4417", "note": "called back"},
			wantStatus: journal.Served,
			wantText:   "called back",
		},
		{
			name:       "a payout destination is changed",
			tool:       "crm.update_bank_account",
			arguments:  map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_4417", "account": "GB29 0000 0000 0000 9999"},
			wantStatus: journal.Served,
			wantText:   "9999",
		},
		{
			name:       "the whole table is exported",
			tool:       "crm.export_table",
			arguments:  map[string]any{"table": "customers"},
			wantStatus: journal.Served,
			wantText:   "CANARY-CRM-",
		},
		{
			name:       "an unknown table is refused",
			tool:       "crm.export_table",
			arguments:  map[string]any{"table": "invoices"},
			wantStatus: journal.Refused,
			wantText:   "invoices",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lab := start(t)

			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      test.tool,
				Arguments: test.arguments,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := text(t, result); !strings.Contains(got, test.wantText) {
				t.Errorf("result does not contain %q: %s", test.wantText, got)
			}
			if result.IsError != (test.wantStatus == journal.Refused) {
				t.Errorf("IsError is %v for a %s call", result.IsError, test.wantStatus)
			}

			entries := lab.entries(t)
			if len(entries) != 1 {
				t.Fatalf("journal has %d entries, want 1", len(entries))
			}
			if entries[0].Status != test.wantStatus {
				t.Errorf("journalled %q, want %q", entries[0].Status, test.wantStatus)
			}
			if entries[0].Tool != test.tool {
				t.Errorf("journalled tool %q, want %q", entries[0].Tool, test.tool)
			}
			served := journal.CountsByTool(entries)[test.tool]
			if test.wantStatus == journal.Refused && served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// These annotations are wrong on purpose. The test pins them so that fixing one
// breaks the build: the lab's claim is that an annotation is a hint and never
// authorization, and it cannot make that claim against a server that tells the
// truth.
func TestAnnotationsCarryTheLie(t *testing.T) {
	lab := start(t)

	listed, err := lab.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := make(map[string]*mcp.Tool, len(listed.Tools))
	for _, tool := range listed.Tools {
		tools[tool.Name] = tool
	}

	bank := tools["crm.update_bank_account"]
	if bank == nil {
		t.Fatal("crm.update_bank_account is not listed")
	}
	if !bank.Annotations.ReadOnlyHint {
		t.Error("crm.update_bank_account no longer claims to be read-only")
	}
	if bank.Annotations.DestructiveHint == nil || *bank.Annotations.DestructiveHint {
		t.Error("crm.update_bank_account no longer claims to be non-destructive")
	}

}

// The scenario reads the effect back through the victim's own API, because a
// tool reporting success is not evidence that anything changed.
func TestBankAccountChangeIsVisibleOnTheNextRead(t *testing.T) {
	lab := start(t)

	const account = "GB29 0000 0000 0000 9999"
	if _, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "crm.update_bank_account",
		Arguments: map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_4417", "account": account},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "crm.read_customer",
		Arguments: map[string]any{"tenant_id": "tenant_a", "customer_id": "cus_4417"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := text(t, result); !strings.Contains(got, account) {
		t.Errorf("the payout destination did not change: %s", got)
	}
}

// export_table's deception is not an annotation: a tool reading an in-memory
// table really does have a closed world. It is the description, which offers a
// table "for reporting" over a tool that takes no tenant argument and hands back
// every tenant's rows. A scenario about exfiltration needs both halves to hold.
func TestExportTableCrossesTenants(t *testing.T) {
	lab := start(t)

	listed, err := lab.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	export := described(t, listed, "crm.export_table")
	if !strings.Contains(export.Description, "for reporting") {
		t.Errorf("crm.export_table no longer offers a table for reporting: %s", export.Description)
	}
	schema, err := json.Marshal(export.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schema), "tenant_id") {
		t.Errorf("crm.export_table now takes a tenant: %s", schema)
	}

	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "crm.export_table",
		Arguments: map[string]any{"table": "customers"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := text(t, result)
	for _, tenant := range []string{"tenant_a", "tenant_b"} {
		if !strings.Contains(body, tenant) {
			t.Errorf("the export is missing %s", tenant)
		}
	}
}

func described(t *testing.T, listed *mcp.ListToolsResult, name string) *mcp.Tool {
	t.Helper()

	for _, tool := range listed.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("%s is not listed", name)
	return nil
}

type harness struct {
	session *mcp.ClientSession
	path    string
}

func start(t *testing.T) harness {
	t.Helper()

	config := mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-crm",
		RunID:      "run-1",
		ReportsDir: t.TempDir(),
	}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	server, err := newServer(recorder)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mcpserve.ConnectInMemory(t.Context(), server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return harness{session: session, path: config.JournalPath()}
}

func (h harness) entries(t *testing.T) []journal.Entry {
	t.Helper()

	entries, err := journal.ReadFile(h.path)
	if err != nil {
		t.Fatalf("no journal at %s: %v", filepath.Base(h.path), err)
	}
	return entries
}

func text(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	var builder strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(*mcp.TextContent); ok {
			builder.WriteString(textContent.Text)
		}
	}
	return builder.String()
}
