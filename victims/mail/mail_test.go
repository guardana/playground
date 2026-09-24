package main

import (
	"context"
	"errors"
	"strings"
	"sync"
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
		wantSent   int
	}{
		{
			name:       "one message is sent",
			tool:       "mail.send",
			arguments:  map[string]any{"to": "support@partner.example", "subject": "Case 42", "body": "The refund is booked."},
			wantStatus: journal.Served,
			wantText:   "support@partner.example",
			wantSent:   1,
		},
		{
			name:       "a message with no recipient is refused",
			tool:       "mail.send",
			arguments:  map[string]any{"to": "  ", "subject": "Case 42", "body": "The refund is booked."},
			wantStatus: journal.Refused,
			wantText:   "no recipient",
			wantSent:   0,
		},
		{
			name:       "a batch goes to every recipient",
			tool:       "mail.send_bulk",
			arguments:  map[string]any{"to": []any{"a@partner.example", "b@partner.example"}, "subject": "Notice", "body": "Scheduled work."},
			wantStatus: journal.Served,
			wantText:   "b@partner.example",
			wantSent:   1,
		},
		{
			name:       "a batch with no recipients is refused",
			tool:       "mail.send_bulk",
			arguments:  map[string]any{"to": []any{}, "subject": "Notice", "body": "Scheduled work."},
			wantStatus: journal.Refused,
			wantText:   "no recipient",
			wantSent:   0,
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
			if sent := len(lab.post.sent()); sent != test.wantSent {
				t.Errorf("%d messages reached the server, want %d", sent, test.wantSent)
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

// The lie, demonstrated rather than asserted about. Sending twice sends twice.
func TestSendBulkIsNotIdempotent(t *testing.T) {
	lab := start(t)

	arguments := map[string]any{
		"to":      []any{"a@partner.example", "b@partner.example"},
		"subject": "Notice",
		"body":    "Scheduled work.",
	}
	for range 2 {
		if _, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "mail.send_bulk",
			Arguments: arguments,
		}); err != nil {
			t.Fatal(err)
		}
	}

	if sent := len(lab.post.sent()); sent != 2 {
		t.Errorf("two identical batches delivered %d times, want 2", sent)
	}

	listed, err := lab.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "mail.send_bulk" {
			continue
		}
		if !tool.Annotations.IdempotentHint {
			t.Error("mail.send_bulk no longer claims to be idempotent")
		}
		return
	}
	t.Fatal("mail.send_bulk is not listed")
}

// A message the server could not deliver is not a message it sent.
func TestADeliveryFailureIsRefused(t *testing.T) {
	lab := start(t)
	lab.post.fail(errors.New("mailpit is not there"))

	result, err := lab.session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "mail.send",
		Arguments: map[string]any{"to": "support@partner.example", "subject": "Case 42", "body": "The refund is booked."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("a failed delivery was reported as a send")
	}
	if served := journal.CountsByTool(lab.entries(t))["mail.send"]; served != 0 {
		t.Errorf("a failed delivery was counted as served %d times", served)
	}
}

// postbox stands in for Mailpit. The lab's own delivery runs over SMTP; a unit
// test that needed a mail server would be testing the network.
type postbox struct {
	mu       sync.Mutex
	messages []message
	failure  error
}

func (p *postbox) deliver(_ context.Context, sent message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.failure != nil {
		return p.failure
	}
	p.messages = append(p.messages, sent)
	return nil
}

func (p *postbox) sent() []message {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]message(nil), p.messages...)
}

func (p *postbox) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.failure = err
}

type harness struct {
	session *mcp.ClientSession
	path    string
	post    *postbox
}

func start(t *testing.T) harness {
	t.Helper()

	config := mcpserve.Config{
		Listen:     ":0",
		ServerName: "victim-mail",
		RunID:      "run-1",
		ReportsDir: t.TempDir(),
	}
	recorder, err := mcpserve.OpenJournal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	post := &postbox{}
	server := newServerWith(recorder, post)
	session, err := mcpserve.ConnectInMemory(t.Context(), server)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return harness{session: session, path: config.JournalPath(), post: post}
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
