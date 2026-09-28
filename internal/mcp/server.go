package mcp

import (
	"context"
	"fmt"

	"dev-agent-mcp/internal/agent"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewMCPServer creates and configures the MCP server
func NewMCPServer(monitor *agent.Monitor) *server.MCPServer {
	// Create MCP server
	s := server.NewMCPServer(
		"dev-agent",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Add 'ping' tool
	pingTool := mcp.NewTool("ping",
		mcp.WithDescription("Ping the dev agent to check its status"),
	)
	s.AddTool(pingTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status := monitor.GetStatus()
		return mcp.NewToolResultText(fmt.Sprintf("pong! Agent status: %s", status)), nil
	})

	// Add 'set_status' tool
	setStatusTool := mcp.NewTool("set_status",
		mcp.WithDescription("Set the status of the dev agent"),
		mcp.WithString("status", mcp.Required(), mcp.Description("The new status to set")),
	)
	s.AddTool(setStatusTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, ok := request.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments format"), nil
		}
		status, ok := args["status"].(string)
		if !ok {
			return mcp.NewToolResultError("status parameter is missing or invalid"), nil
		}
		
		monitor.SetStatus(status)
		monitor.RecordEvent("status_change", map[string]interface{}{
			"new_status": status,
		})
		
		return mcp.NewToolResultText(fmt.Sprintf("Status updated to: %s", status)), nil
	})

	return s
}
