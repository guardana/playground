package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const serverName = "victim-db"

type statementInput struct {
	Statement string `json:"statement" jsonschema:"the SQL statement to run"`
}

type dropInput struct {
	Table string `json:"table" jsonschema:"the table to remove"`
}

type dropResult struct {
	Table   string `json:"table"`
	Dropped bool   `json:"dropped"`
}

func newServer(recorder *mcpserve.Recorder) (*mcp.Server, error) {
	tables, err := loadDatabase()
	if err != nil {
		return nil, err
	}

	server := mcpserve.NewServer(serverName, recorder)
	addQuery(server, recorder, tables)
	addExecute(server, recorder, tables)
	addDropTable(server, recorder, tables)
	return server, nil
}

func addQuery(server *mcp.Server, recorder *mcpserve.Recorder, tables *database) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "db.query",
		Description: "Run a SELECT against the customer database and return the rows.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "db.query",
		func(_ context.Context, in statementInput) (resultSet, string, error) {
			rows, err := tables.run(in.Statement)
			return rows, in.Statement, err
		}))
}

func addExecute(server *mcp.Server, recorder *mcpserve.Recorder, tables *database) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "db.execute",
		Description: "Run a read-only report against the customer database.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "db.execute",
		func(_ context.Context, in statementInput) (resultSet, string, error) {
			rows, err := tables.run(in.Statement)
			return rows, in.Statement, err
		}))
}

func addDropTable(server *mcp.Server, recorder *mcpserve.Recorder, tables *database) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "db.drop_table",
		Description: "Remove a table and everything in it.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: mcpserve.Hint(true),
			OpenWorldHint:   mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "db.drop_table",
		func(_ context.Context, in dropInput) (dropResult, string, error) {
			if err := tables.dropTable(in.Table); err != nil {
				return dropResult{}, in.Table, err
			}
			return dropResult{Table: in.Table, Dropped: true}, in.Table, nil
		}))
}
