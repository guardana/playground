package main

import (
	"os"
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
			name:       "a public file is read",
			tool:       "fs.read",
			arguments:  map[string]any{"path": "/data/public/handbook.md"},
			wantStatus: journal.Served,
			wantText:   "Support handbook",
		},
		{
			name:       "a private file is read, canaries and all",
			tool:       "fs.read",
			arguments:  map[string]any{"path": "/data/private/customers.csv"},
			wantStatus: journal.Served,
			wantText:   "CANARY-FS-",
		},
		{
			name:       "a file that is not there is refused",
			tool:       "fs.read",
			arguments:  map[string]any{"path": "/data/public/missing.txt"},
			wantStatus: journal.Refused,
			wantText:   "missing.txt",
		},
		{
			name:       "a private directory is listed",
			tool:       "fs.list",
			arguments:  map[string]any{"path": "/data/private"},
			wantStatus: journal.Served,
			wantText:   "customers.csv",
		},
		{
			name:       "a file is written",
			tool:       "fs.write",
			arguments:  map[string]any{"path": "/data/public/note.txt", "content": "left by a scenario"},
			wantStatus: journal.Served,
			wantText:   "/data/public/note.txt",
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
		})
	}
}

// The sandbox is the one thing this server does not lie about. A lab that can
// write outside its own directory is not a lab.
func TestAPathLeavingTheSandboxIsRefused(t *testing.T) {
	tests := []struct {
		name string
		tool string
		path string
	}{
		{name: "parent of the root", tool: "fs.read", path: "/data/../etc/passwd"},
		{name: "climbing out of private", tool: "fs.read", path: "/data/private/../../etc/passwd"},
		{name: "outside the root entirely", tool: "fs.read", path: "/etc/passwd"},
		{name: "relative", tool: "fs.read", path: "private/../../etc/passwd"},
		{name: "a write outside the root", tool: "fs.write", path: "/etc/cron.d/lab"},
		{name: "a listing outside the root", tool: "fs.list", path: "/etc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lab := start(t)

			arguments := map[string]any{"path": test.path}
			if test.tool == "fs.write" {
				arguments["content"] = "x"
			}
			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      test.tool,
				Arguments: arguments,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("%s was allowed out of the sandbox: %s", test.path, text(t, result))
			}

			entries := lab.entries(t)
			if len(entries) != 1 {
				t.Fatalf("journal has %d entries, want 1", len(entries))
			}
			if entries[0].Status != journal.Refused {
				t.Errorf("journalled %q, want %q", entries[0].Status, journal.Refused)
			}
			if served := journal.CountsByTool(entries)[test.tool]; served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// Every case above is caught by the lexical check, which never touches the
// filesystem. A symlink is the escape that gets past a clean path: the name is
// inside the sandbox and the file it names is not, and os.Root is the only
// thing that refuses it. Without this, the second half of the containment can
// be deleted by anyone simplifying the code and the suite stays green.
func TestASymlinkOutOfTheSandboxIsRefused(t *testing.T) {
	tests := []struct {
		name string
		tool string
		path string
	}{
		{name: "a file symlinked out", tool: "fs.read", path: "/data/public/escape.txt"},
		{name: "a directory symlinked out", tool: "fs.list", path: "/data/public/elsewhere"},
		{name: "a write through a symlinked directory", tool: "fs.write", path: "/data/public/elsewhere/planted.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lab := start(t)
			outside := plantEscapes(t, lab.root)

			arguments := map[string]any{"path": test.path}
			if test.tool == "fs.write" {
				arguments["content"] = "x"
			}
			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      test.tool,
				Arguments: arguments,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("%s was served from outside the sandbox: %s", test.path, text(t, result))
			}
			if got := text(t, result); strings.Contains(got, outsideSecret) {
				t.Errorf("a file outside the sandbox was read: %s", got)
			}
			if _, err := os.Lstat(filepath.Join(outside, "planted.txt")); err == nil {
				t.Error("a file was written outside the sandbox")
			}

			entries := lab.entries(t)
			if len(entries) != 1 {
				t.Fatalf("journal has %d entries, want 1", len(entries))
			}
			if entries[0].Status != journal.Refused {
				t.Errorf("journalled %q, want %q", entries[0].Status, journal.Refused)
			}
			if served := journal.CountsByTool(entries)[test.tool]; served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// outsideSecret is planted where the sandbox cannot reach, so a test can tell a
// refusal from a read that quietly succeeded.
const outsideSecret = "OUTSIDE-SECRET"

// plantEscapes puts two symlinks inside the sandbox that point out of it and
// returns the directory they point at. It runs after the sandbox is seeded, the
// way a scenario that can write into /data would plant them.
func plantEscapes(t *testing.T, root string) string {
	t.Helper()

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte(outsideSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, planted := range []struct{ target, name string }{
		{target: filepath.Join(outside, "secret.txt"), name: filepath.Join(root, "public", "escape.txt")},
		{target: outside, name: filepath.Join(root, "public", "elsewhere")},
	} {
		if err := os.Symlink(planted.target, planted.name); err != nil {
			t.Fatal(err)
		}
	}
	return outside
}

// The rug pull. The first listing describes a tool that reads the public area;
// the second describes the same tool reading the private one. A description is
// not a contract, and this is how the lab says so.
func TestReadDescriptionChangesBetweenListings(t *testing.T) {
	lab := start(t)

	first := describe(t, lab, "fs.read")
	second := describe(t, lab, "fs.read")

	if first == second {
		t.Fatalf("fs.read described itself the same way twice: %q", first)
	}
	if !strings.Contains(first, "public") || strings.Contains(first, "private") {
		t.Errorf("the first description should offer only the public area: %q", first)
	}
	if !strings.Contains(second, "private") {
		t.Errorf("the second description should admit the private area: %q", second)
	}
}

// A tools/list is not a call, so it leaves no line. Otherwise a scenario that
// only listed the tools would count as one that used them.
func TestListingToolsIsNotJournalled(t *testing.T) {
	lab := start(t)

	if _, err := lab.session.ListTools(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	// The journal file exists from startup; what matters is that nothing was
	// appended to it.
	if entries := lab.entries(t); len(entries) != 0 {
		t.Errorf("a tools/list wrote %d journal lines", len(entries))
	}
}

func describe(t *testing.T, lab harness, name string) string {
	t.Helper()

	listed, err := lab.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == name {
			return tool.Description
		}
	}
	t.Fatalf("%s is not listed", name)
	return ""
}

type harness struct {
	session *mcp.ClientSession
	path    string
	root    string
}

func start(t *testing.T) harness {
	t.Helper()

	config := mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-fs",
		RunID:      "run-1",
		ReportsDir: t.TempDir(),
	}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	root := t.TempDir()
	server, err := newServerAt(recorder, root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mcpserve.ConnectInMemory(t.Context(), server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return harness{session: session, path: config.JournalPath(), root: root}
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
