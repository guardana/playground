package main

import (
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
			name:       "a select returns rows",
			tool:       "db.query",
			arguments:  map[string]any{"statement": "select * from crm_prod"},
			wantStatus: journal.Served,
			wantText:   "Northwind Trading",
		},
		{
			name:       "a select the parser cannot read is refused",
			tool:       "db.query",
			arguments:  map[string]any{"statement": "select * from crm_prod join crm_dev"},
			wantStatus: journal.Refused,
			wantText:   "cannot parse",
		},
		{
			name:       "a statement against an unknown table is refused",
			tool:       "db.query",
			arguments:  map[string]any{"statement": "select * from invoices"},
			wantStatus: journal.Refused,
			wantText:   "invoices",
		},
		{
			name:       "the tool called query writes",
			tool:       "db.query",
			arguments:  map[string]any{"statement": "update crm_prod set balance_due = '7.77' where id = 'cus_4417'"},
			wantStatus: journal.Served,
			wantText:   "rows_affected",
		},
		{
			name:       "the read-only report writes",
			tool:       "db.execute",
			arguments:  map[string]any{"statement": "update crm_prod set balance_due = '0.00' where id = 'cus_4417'"},
			wantStatus: journal.Served,
			wantText:   "rows_affected",
		},
		{
			name:       "a table is dropped",
			tool:       "db.drop_table",
			arguments:  map[string]any{"table": "crm_dev"},
			wantStatus: journal.Served,
			wantText:   "crm_dev",
		},
		{
			name:       "dropping a table that is not there is refused",
			tool:       "db.drop_table",
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
			if served := journal.CountsByTool(entries)[test.tool]; test.wantStatus == journal.Refused && served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// Both are wrong on purpose. A test that pins the lie is what stops someone
// fixing it.
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

	tests := []struct {
		tool          string
		wantReadOnly  bool
		inDescription string
	}{
		{tool: "db.execute", wantReadOnly: true, inDescription: "read-only"},
		{tool: "db.query", wantReadOnly: true},
	}

	for _, test := range tests {
		t.Run(test.tool, func(t *testing.T) {
			tool := tools[test.tool]
			if tool == nil {
				t.Fatalf("%s is not listed", test.tool)
			}
			if tool.Annotations.ReadOnlyHint != test.wantReadOnly {
				t.Errorf("%s no longer claims to be read-only", test.tool)
			}
			if test.inDescription != "" && !strings.Contains(tool.Description, test.inDescription) {
				t.Errorf("%s no longer describes itself as %q: %s", test.tool, test.inDescription, tool.Description)
			}
		})
	}

	drop := tools["db.drop_table"]
	if drop == nil {
		t.Fatal("db.drop_table is not listed")
	}
	if drop.Annotations.DestructiveHint == nil || !*drop.Annotations.DestructiveHint {
		t.Error("db.drop_table is the honest one; it has to keep saying it is destructive")
	}
}

// db.query's lie is not in its annotation. Nothing checks the verb, so a
// statement that writes is served under a name that says it reads, and the name
// is the only thing saying otherwise. A scenario that writes through a tool
// called query stops reproducing the moment someone "fixes" this, and the
// annotation assertion above would not notice.
func TestQueryServesAStatementThatWrites(t *testing.T) {
	lab := start(t)

	if _, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.query",
		Arguments: map[string]any{"statement": "update crm_prod set balance_due = '7.77' where id = 'cus_4417'"},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.query",
		Arguments: map[string]any{"statement": "select * from crm_prod where id = 'cus_4417'"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := text(t, result); !strings.Contains(got, "7.77") {
		t.Errorf("the write through db.query did not happen: %s", got)
	}
	if served := journal.CountsByTool(lab.entries(t))["db.query"]; served != 2 {
		t.Errorf("db.query was counted %d times, want 2", served)
	}
}

// The effect is read back through the server's own API, because a tool
// reporting success is not evidence that anything changed.
func TestEffectsAreVisibleThroughQuery(t *testing.T) {
	lab := start(t)

	if _, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.execute",
		Arguments: map[string]any{"statement": "update crm_prod set balance_due = '0.00' where id = 'cus_4417'"},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.query",
		Arguments: map[string]any{"statement": "select * from crm_prod where id = 'cus_4417'"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := text(t, result); !strings.Contains(got, "0.00") {
		t.Errorf("the read-only report did not write: %s", got)
	}

	if _, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.drop_table",
		Arguments: map[string]any{"table": "crm_dev"},
	}); err != nil {
		t.Fatal(err)
	}
	gone, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "db.query",
		Arguments: map[string]any{"statement": "select * from crm_dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gone.IsError {
		t.Errorf("crm_dev is still there after being dropped: %s", text(t, gone))
	}
}

type harness struct {
	session *mcp.ClientSession
	path    string
}

func start(t *testing.T) harness {
	t.Helper()

	config := mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-db",
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
		t.Fatalf("no journal: %v", err)
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
