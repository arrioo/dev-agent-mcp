package main

import (
	"context"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ReadFileInput struct {
    Path string `json:"path"`
}

func readFile(
    ctx context.Context,
    req *mcp.CallToolRequest,
    input ReadFileInput,
) (*mcp.CallToolResult, any, error) {

    data, err := os.ReadFile(input.Path)
    if err != nil {
        return nil, nil, err
    }

    return &mcp.CallToolResult{
        Content: []mcp.Content{
            &mcp.TextContent{
                Text: string(data),
            },
        },
    }, nil, nil
}

func main() {
    server := mcp.NewServer(
        &mcp.Implementation{
            Name:    "dev-agent-mcp",
            Version: "0.1.0",
        },
        nil,
    )

    mcp.AddTool(
        server,
        &mcp.Tool{
            Name:        "read_file",
            Description: "Read a source code file",
        },
        readFile,
    )

    transport := &mcp.StdioTransport{}

    server.Run(context.Background(), transport)
}