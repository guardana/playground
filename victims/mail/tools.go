package main

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const serverName = "victim-mail"

type sendInput struct {
	To      string `json:"to" jsonschema:"the recipient"`
	Subject string `json:"subject" jsonschema:"the subject line"`
	Body    string `json:"body" jsonschema:"the message body"`
}

type bulkInput struct {
	To      []string `json:"to" jsonschema:"the recipients"`
	Subject string   `json:"subject" jsonschema:"the subject line"`
	Body    string   `json:"body" jsonschema:"the message body"`
}

type sendResult struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Bytes   int      `json:"bytes"`
}

func newServer(recorder *mcpserve.Recorder) (*mcp.Server, error) {
	return newServerWith(recorder, mailpit{address: mailpitAddress}), nil
}

// newServerWith takes the deliverer so a test can count what was sent without
// running a mail server.
func newServerWith(recorder *mcpserve.Recorder, post deliverer) *mcp.Server {
	server := mcpserve.NewServer(serverName, recorder)
	addSend(server, recorder, post)
	addSendBulk(server, recorder, post)
	return server
}

func addSend(server *mcp.Server, recorder *mcpserve.Recorder, post deliverer) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail.send",
		Description: "Send one message to one recipient.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(false),
			OpenWorldHint:   mcpserve.Hint(true),
		},
	}, mcpserve.Journalled(recorder, "mail.send",
		func(ctx context.Context, in sendInput) (sendResult, string, error) {
			return deliver(ctx, post, recipients(in.To), in.Subject, in.Body)
		}))
}

func addSendBulk(server *mcp.Server, recorder *mcpserve.Recorder, post deliverer) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mail.send_bulk",
		Description: "Send one message to a list of recipients.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(false),
			IdempotentHint:  true,
			OpenWorldHint:   mcpserve.Hint(true),
		},
	}, mcpserve.Journalled(recorder, "mail.send_bulk",
		func(ctx context.Context, in bulkInput) (sendResult, string, error) {
			return deliver(ctx, post, recipients(in.To...), in.Subject, in.Body)
		}))
}

func deliver(ctx context.Context, post deliverer, to []string, subject, body string) (sendResult, string, error) {
	detail := strings.Join(to, ", ")
	if len(to) == 0 {
		return sendResult{}, detail, errNoRecipient
	}
	if err := post.deliver(ctx, message{To: to, Subject: subject, Body: body}); err != nil {
		return sendResult{}, detail, err
	}
	return sendResult{To: to, Subject: subject, Bytes: len(body)}, detail, nil
}

// recipients drops the empty strings, so a call with a blank address is
// refused rather than delivered to nobody and reported as sent.
func recipients(given ...string) []string {
	var kept []string
	for _, address := range given {
		if trimmed := strings.TrimSpace(address); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return kept
}
