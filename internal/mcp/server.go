// Package mcp exposes Mimikos as an MCP (Model Context Protocol) server so AI
// agents can start, query, and manage mock servers via tool calls. The package
// constructs an mcp.Server with registered tool handlers and delegates to an
// Instance for mock server lifecycle management.
package mcp

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP protocol server and the Mimikos instance it manages.
// Create one with NewServer and run it with Run.
type Server struct {
	mcpServer *mcp.Server
	instance  *Instance
	logger    *slog.Logger
}

// NewServer creates an MCP server with all Mimikos tools registered.
// The version string is embedded in the MCP server implementation metadata.
func NewServer(version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	impl := &mcp.Implementation{
		Name:    "mimikos",
		Version: version,
	}

	mcpSrv := mcp.NewServer(impl, &mcp.ServerOptions{
		Logger: logger,
	})

	inst := &Instance{}

	s := &Server{
		mcpServer: mcpSrv,
		instance:  inst,
		logger:    logger,
	}

	s.registerTools()

	return s
}

// Run starts the MCP server on the given transport and blocks until the
// context is cancelled or the transport is closed. For CLI usage, pass
// &mcp.StdioTransport{}.
func (s *Server) Run(ctx context.Context, transport mcp.Transport) error {
	return s.mcpServer.Run(ctx, transport)
}

// registerTools adds all Mimikos tool handlers to the MCP server.
func (s *Server) registerTools() {
	s.registerStartServer()
	s.registerStopServer()
	s.registerServerStatus()
	s.registerListEndpoints()
	s.registerGetEndpoint()
	s.registerManageState()
	s.registerRequestStatus()
	s.registerGetRequestLog()
}
