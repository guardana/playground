package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"
)

const (
	// mailpitAddress is where the lab's mail goes. Mailpit accepts everything
	// and delivers nowhere, which is what a lab needs from a mail server.
	mailpitAddress = "mailpit:1025"
	sender         = "support@lab.example"
	deliveryLimit  = 10 * time.Second
)

var errNoRecipient = errors.New("no recipient")

// message is one delivery. A batch is one message with several recipients,
// which is why sending the same batch twice delivers it twice.
type message struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
}

// deliverer is what stands between the tools and the network, so a test can
// count what was sent without running a mail server.
type deliverer interface {
	deliver(ctx context.Context, sent message) error
}

// mailpit delivers over SMTP with no authentication and no encryption. Both
// are absent because the lab's network has no route out; on anything else this
// would be a defect.
type mailpit struct {
	address string
}

func (m mailpit) deliver(ctx context.Context, sent message) error {
	if len(sent.To) == 0 {
		return errNoRecipient
	}

	ctx, cancel := context.WithTimeout(ctx, deliveryLimit)
	defer cancel()

	var dialer net.Dialer
	connection, err := dialer.DialContext(ctx, "tcp", m.address)
	if err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		// The context covers the dial; this covers the conversation after it.
		if err := connection.SetDeadline(deadline); err != nil {
			return errors.Join(err, connection.Close())
		}
	}

	host, _, err := net.SplitHostPort(m.address)
	if err != nil {
		return errors.Join(err, connection.Close())
	}
	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return errors.Join(err, connection.Close())
	}
	defer func() { _ = client.Close() }()

	return send(client, sent)
}

func send(client *smtp.Client, sent message) error {
	if err := client.Mail(sender); err != nil {
		return err
	}
	for _, recipient := range sent.To {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(writer, render(sent)); err != nil {
		return errors.Join(err, writer.Close())
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func render(sent message) string {
	var body strings.Builder
	fmt.Fprintf(&body, "From: %s\r\n", sender)
	fmt.Fprintf(&body, "To: %s\r\n", strings.Join(sent.To, ", "))
	fmt.Fprintf(&body, "Subject: %s\r\n", sent.Subject)
	fmt.Fprintf(&body, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	body.WriteString("\r\n")
	body.WriteString(sent.Body)
	return body.String()
}
