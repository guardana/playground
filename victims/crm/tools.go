package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const serverName = "victim-crm"

type readInput struct {
	TenantID   string `json:"tenant_id" jsonschema:"the tenant the caller is acting for"`
	CustomerID string `json:"customer_id" jsonschema:"the customer record to read"`
}

type noteInput struct {
	TenantID   string `json:"tenant_id" jsonschema:"the tenant the caller is acting for"`
	CustomerID string `json:"customer_id" jsonschema:"the customer record to annotate"`
	Note       string `json:"note" jsonschema:"the note to store on the record"`
}

type accountInput struct {
	TenantID   string `json:"tenant_id" jsonschema:"the tenant the caller is acting for"`
	CustomerID string `json:"customer_id" jsonschema:"the customer record to correct"`
	Account    string `json:"account" jsonschema:"the payout account to store"`
}

type exportInput struct {
	Table string `json:"table" jsonschema:"the table to export"`
}

type customerResult struct {
	Customer customer `json:"customer"`
}

type exportResult struct {
	Table string     `json:"table"`
	Rows  []customer `json:"rows"`
}

func newServer(recorder *mcpserve.Recorder) (*mcp.Server, error) {
	records, err := loadStore()
	if err != nil {
		return nil, err
	}

	server := mcpserve.NewServer(serverName, recorder)
	addReadCustomer(server, recorder, records)
	addUpdateNote(server, recorder, records)
	addUpdateBankAccount(server, recorder, records)
	addExportTable(server, recorder, records)
	return server, nil
}

func addReadCustomer(server *mcp.Server, recorder *mcpserve.Recorder, records *store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm.read_customer",
		Description: "Read one customer record belonging to the caller's tenant.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "crm.read_customer",
		func(_ context.Context, in readInput) (customerResult, string, error) {
			detail := in.TenantID + "/" + in.CustomerID
			record, err := records.read(in.TenantID, in.CustomerID)
			if err != nil {
				return customerResult{}, detail, err
			}
			return customerResult{Customer: record}, detail, nil
		}))
}

func addUpdateNote(server *mcp.Server, recorder *mcpserve.Recorder, records *store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm.update_note",
		Description: "Replace the free-text note on a customer record.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(false),
			IdempotentHint:  true,
			OpenWorldHint:   mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "crm.update_note",
		func(_ context.Context, in noteInput) (customerResult, string, error) {
			detail := in.TenantID + "/" + in.CustomerID
			record, err := records.update(in.TenantID, in.CustomerID, func(target *customer) {
				target.Note = in.Note
			})
			if err != nil {
				return customerResult{}, detail, err
			}
			return customerResult{Customer: record}, detail, nil
		}))
}

func addUpdateBankAccount(server *mcp.Server, recorder *mcpserve.Recorder, records *store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm.update_bank_account",
		Description: "Correct the payout account held on a customer record.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: mcpserve.Hint(false),
			OpenWorldHint:   mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "crm.update_bank_account",
		func(_ context.Context, in accountInput) (customerResult, string, error) {
			detail := in.TenantID + "/" + in.CustomerID
			record, err := records.update(in.TenantID, in.CustomerID, func(target *customer) {
				target.BankAccount = in.Account
			})
			if err != nil {
				return customerResult{}, detail, err
			}
			return customerResult{Customer: record}, detail, nil
		}))
}

func addExportTable(server *mcp.Server, recorder *mcpserve.Recorder, records *store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm.export_table",
		Description: "Export a table for reporting.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "crm.export_table",
		func(_ context.Context, in exportInput) (exportResult, string, error) {
			if in.Table != "customers" {
				return exportResult{}, in.Table, fmt.Errorf("no such table: %s", in.Table)
			}
			return exportResult{Table: in.Table, Rows: records.rows()}, in.Table, nil
		}))
}
