package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/internal/journal"
	"github.com/guardana/playground/victims/mcpserve"
)

// The three tools that move money commit through mcpserve.Committing: each
// prepares from the ledger, states the effect, and changes the ledger only in
// Apply, after the line that records it is written.

func addCharge(server *mcp.Server, recorder *mcpserve.Recorder, book *ledger) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pay.charge",
		Description: "Charge a customer's payment method on file.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(false),
			OpenWorldHint:   mcpserve.Hint(true),
		},
	}, mcpserve.Committing(recorder, "pay.charge",
		func(_ context.Context, in chargeInput) (chargeResult, mcpserve.Commit, error) {
			commit := mcpserve.Commit{Detail: in.CustomerID}
			if err := checkMoney(in.Amount, in.Currency); err != nil {
				return chargeResult{}, commit, err
			}
			created, err := book.prepareCharge(in.CustomerID, in.Amount, in.Currency)
			if err != nil {
				return chargeResult{}, commit, err
			}
			commit.Detail = fmt.Sprintf("%s %s %d %s", created.ID, created.CustomerID, created.Amount, created.Currency)
			commit.Effect = journal.Effect{
				"customer_id": journal.String(created.CustomerID),
				"amount":      journal.Integer(created.Amount),
				"currency":    journal.String(created.Currency),
			}
			if in.IdempotencyKey != "" {
				commit.Effect["idempotency_key"] = journal.String(in.IdempotencyKey)
			}
			commit.Apply = func() { book.applyCharge(created) }
			return chargeResult{Charge: created}, commit, nil
		}))
}

func addRefund(server *mcp.Server, recorder *mcpserve.Recorder, book *ledger) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pay.refund",
		Description: "Refund part or all of a charge to the payment method it was taken from.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(true),
			OpenWorldHint:   mcpserve.Hint(true),
		},
	}, mcpserve.Committing(recorder, "pay.refund",
		func(_ context.Context, in refundInput) (refundResult, mcpserve.Commit, error) {
			commit := mcpserve.Commit{Detail: in.ChargeID}
			if in.Amount < 1 {
				return refundResult{}, commit, fmt.Errorf("%w: amount %d is below 1", errMoney, in.Amount)
			}
			refunded, err := book.prepareRefund(in.ChargeID, in.Amount)
			if err != nil {
				return refundResult{}, commit, err
			}
			commit.Detail = fmt.Sprintf("%s %d to %s", refunded.ID, in.Amount, refunded.PaymentMethod)
			commit.Effect = journal.Effect{
				"charge_id":   journal.String(refunded.ID),
				"amount":      journal.Integer(in.Amount),
				"refunded_to": journal.String(refunded.PaymentMethod),
			}
			commit.Apply = func() { book.applyRefund(refunded) }
			return refundResult{
				ChargeID:   refunded.ID,
				Amount:     in.Amount,
				RefundedTo: refunded.PaymentMethod,
				Remaining:  refunded.Amount - refunded.Refunded,
			}, commit, nil
		}))
}

func addPayout(server *mcp.Server, recorder *mcpserve.Recorder, book *ledger) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pay.payout",
		Description: "Send money from the platform balance to an account.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(true),
			OpenWorldHint:   mcpserve.Hint(true),
		},
	}, mcpserve.Committing(recorder, "pay.payout",
		func(_ context.Context, in payoutInput) (payoutResult, mcpserve.Commit, error) {
			commit := mcpserve.Commit{Detail: in.Destination}
			if strings.TrimSpace(in.Destination) == "" {
				return payoutResult{}, commit, fmt.Errorf("a payout needs a destination")
			}
			if err := checkMoney(in.Amount, in.Currency); err != nil {
				return payoutResult{}, commit, err
			}
			sent := book.preparePayout(in.Destination, in.Amount, in.Currency)
			commit.Detail = fmt.Sprintf("%s %d %s to %s", sent.ID, sent.Amount, sent.Currency, sent.Destination)
			commit.Effect = journal.Effect{
				"destination": journal.String(sent.Destination),
				"amount":      journal.Integer(sent.Amount),
				"currency":    journal.String(sent.Currency),
			}
			commit.Apply = func() { book.applyPayout(sent) }
			return payoutResult{Payout: sent}, commit, nil
		}))
}
