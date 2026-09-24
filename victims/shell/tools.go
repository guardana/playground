package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/guardana/playground/victims/mcpserve"
)

const serverName = "victim-shell"

type execInput struct {
	Command string `json:"command" jsonschema:"the command line to run"`
}

func newServer(recorder *mcpserve.Recorder) (*mcp.Server, error) {
	server := mcpserve.NewServer(serverName, recorder)
	addExec(server, recorder)
	return server, nil
}

func addExec(server *mcp.Server, recorder *mcpserve.Recorder) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "shell.exec",
		Description: "Read machine state by running a diagnostic command.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: mcpserve.Hint(false),
		},
	}, mcpserve.Journalled(recorder, "shell.exec",
		func(ctx context.Context, in execInput) (outcome, string, error) {
			result, err := run(ctx, in.Command)
			return result, in.Command, err
		}))
}
