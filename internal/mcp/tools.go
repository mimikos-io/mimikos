package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mimikos-io/mimikos/internal/classifier"
)

type (
	// Tool input types — struct tags drive MCP schema inference.
	// The jsonschema tag provides descriptions; json tags control naming.
	// Non-pointer fields without omitempty are automatically required.

	// startServerArgs holds the input parameters for the start_server tool.
	startServerArgs struct {
		SpecPath string `json:"specPath"           jsonschema:"Path to OpenAPI spec file (YAML or JSON)"`
		Mode     string `json:"mode,omitempty"     jsonschema:"Operating mode: deterministic or stateful"`
		Port     int    `json:"port,omitempty"     jsonschema:"TCP port to listen on (default 8080)"`
		Strict   bool   `json:"strict,omitempty"   jsonschema:"Return 500 on response schema validation failure"`
		MaxDepth int    `json:"maxDepth,omitempty" jsonschema:"Max recursion depth for circular schemas (default 10)"`
	}

	// getEndpointArgs holds the input parameters for the get_endpoint tool.
	getEndpointArgs struct {
		Method string `json:"method" jsonschema:"HTTP method (GET, POST, PUT, PATCH, DELETE)"`
		Path   string `json:"path"   jsonschema:"Path pattern (e.g. /pets/{petId})"`
	}

	endpointInfo struct {
		Method     string `json:"method"`
		Path       string `json:"path"`
		Behavior   string `json:"behavior"`
		Confidence string `json:"confidence"`
	}
)

const (
	defaultPort     = 8080
	defaultMaxDepth = 10
)

// registerStartServer registers the start_server tool with the MCP server.
func (s *Server) registerStartServer() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "start_server",
		Description: "Start a Mimikos mock server from an OpenAPI spec. " +
			"Generates realistic API responses with zero configuration.",
	}, s.handleStartServer)
}

// handleStartServer reads the spec file and starts the mock server.
func (s *Server) handleStartServer(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	args startServerArgs,
) (*mcp.CallToolResult, any, error) {
	specBytes, err := os.ReadFile(args.SpecPath)
	if err != nil {
		return toolError(
			"Cannot read spec file %q: %s", args.SpecPath, err,
		), nil, nil
	}

	port := args.Port
	if port == 0 {
		port = defaultPort
	}

	maxDepth := args.MaxDepth
	if maxDepth == 0 {
		maxDepth = defaultMaxDepth
	}

	mode := args.Mode
	if mode == "" {
		mode = "deterministic"
	}

	cfg := StartConfig{
		SpecPath:  args.SpecPath,
		SpecBytes: specBytes,
		Port:      port,
		Strict:    args.Strict,
		MaxDepth:  maxDepth,
	}

	cfg.Mode, err = parseMode(mode)
	if err != nil {
		return toolError("%s", err), nil, nil
	}

	if err := s.instance.Start(ctx, cfg); err != nil {
		return toolError("%s", err), nil, nil
	}

	// Build response with endpoint summary.
	status := s.instance.Status()

	s.instance.mu.Lock()
	entries := s.instance.startupResult.Entries
	s.instance.mu.Unlock()

	endpoints := make([]endpointInfo, 0, status.Operations)
	for _, e := range entries {
		endpoints = append(endpoints, endpointInfo{
			Method:     e.Method,
			Path:       e.PathPattern,
			Behavior:   e.BehaviorType,
			Confidence: e.Confidence,
		})
	}

	result := map[string]any{
		"port":         status.Port,
		"spec_title":   status.SpecTitle,
		"spec_version": status.SpecVersion,
		"operations":   status.Operations,
		"mode":         string(status.Mode),
		"endpoints":    endpoints,
	}

	return toolJSON(result), nil, nil
}

// registerStopServer registers the stop_server tool with the MCP server.
func (s *Server) registerStopServer() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "stop_server",
		Description: "Stop the running Mimikos mock server.",
	}, s.handleStopServer)
}

// handleStopServer gracefully shuts down the mock server.
func (s *Server) handleStopServer(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, any, error) {
	// Capture info before stopping.
	status := s.instance.Status()

	if err := s.instance.Stop(ctx); err != nil {
		return toolError("%s", err), nil, nil
	}

	result := map[string]any{
		"stopped":    true,
		"was_port":   status.Port,
		"was_mode":   string(status.Mode),
		"was_spec":   status.SpecTitle,
		"was_uptime": fmt.Sprintf("%.0fs", status.UptimeSeconds),
	}

	return toolJSON(result), nil, nil
}

// registerServerStatus registers the server_status tool with the MCP server.
func (s *Server) registerServerStatus() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "server_status",
		Description: "Check if a Mimikos server is running and get its details.",
	}, s.handleServerStatus)
}

// handleServerStatus returns the current server state.
func (s *Server) handleServerStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, any, error) {
	status := s.instance.Status()

	if !status.Running {
		result := map[string]any{"running": false}

		return toolJSON(result), nil, nil
	}

	result := map[string]any{
		"running":        true,
		"port":           status.Port,
		"mode":           string(status.Mode),
		"spec_title":     status.SpecTitle,
		"spec_version":   status.SpecVersion,
		"operations":     status.Operations,
		"uptime_seconds": status.UptimeSeconds,
	}

	return toolJSON(result), nil, nil
}

// registerListEndpoints registers the list_endpoints tool with the MCP server.
func (s *Server) registerListEndpoints() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "list_endpoints",
		Description: "List all classified endpoints on the running " +
			"Mimikos server.",
	}, s.handleListEndpoints)
}

// handleListEndpoints returns all endpoint summaries from the behavior map.
func (s *Server) handleListEndpoints(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, any, error) {
	if !s.instance.IsRunning() {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	s.instance.mu.Lock()
	entries := s.instance.behaviorMap.Entries()
	s.instance.mu.Unlock()

	type endpointSummary struct {
		Method     string `json:"method"`
		Path       string `json:"path"`
		Behavior   string `json:"behavior"`
		Confidence string `json:"confidence"`
	}

	endpoints := make([]endpointSummary, 0, len(entries))
	for _, e := range entries {
		endpoints = append(endpoints, endpointSummary{
			Method:     e.Method,
			Path:       e.PathPattern,
			Behavior:   string(e.Type),
			Confidence: classifier.ConfidenceLabel(e.Confidence),
		})
	}

	return toolJSON(endpoints), nil, nil
}

// registerGetEndpoint registers the get_endpoint tool with the MCP server.
func (s *Server) registerGetEndpoint() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_endpoint",
		Description: "Get detailed information about a specific endpoint " +
			"on the running Mimikos server.",
	}, s.handleGetEndpoint)
}

// handleGetEndpoint returns detailed info for a single endpoint.
func (s *Server) handleGetEndpoint(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args getEndpointArgs,
) (*mcp.CallToolResult, any, error) {
	if !s.instance.IsRunning() {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	method := strings.ToUpper(args.Method)

	s.instance.mu.Lock()
	entry, ok := s.instance.behaviorMap.Get(method, args.Path)
	s.instance.mu.Unlock()

	if !ok {
		return toolError(
			"No endpoint found for %s %s. "+
				"Use list_endpoints to see available endpoints.",
			method, args.Path,
		), nil, nil
	}

	hasRespSchema := entry.ResponseSchemas[entry.SuccessCode] != nil
	degraded := entry.DegradedResponseSchema != "" ||
		entry.DegradedRequestSchema != ""

	result := map[string]any{
		"method":                entry.Method,
		"path":                  entry.PathPattern,
		"behavior":              string(entry.Type),
		"confidence":            classifier.ConfidenceLabel(entry.Confidence),
		"success_code":          entry.SuccessCode,
		"has_request_schema":    entry.RequestSchema != nil,
		"has_response_schema":   hasRespSchema,
		"has_response_examples": len(entry.ResponseExamples) > 0,
		"degraded":              degraded,
		"wrapper_key":           entry.WrapperKey,
		"list_array_key":        entry.ListArrayKey,
		"id_field_hint":         entry.IDFieldHint,
		"operation_id":          entry.OperationID,
	}

	if entry.DegradedResponseSchema != "" {
		result["degraded_response_reason"] = entry.DegradedResponseSchema
	}

	if entry.DegradedRequestSchema != "" {
		result["degraded_request_reason"] = entry.DegradedRequestSchema
	}

	return toolJSON(result), nil, nil
}

// toolError creates a CallToolResult with IsError set and a formatted message.
func toolError(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
		IsError: true,
	}
}

// toolJSON creates a CallToolResult with the value marshaled as JSON text.
func toolJSON(v any) *mcp.CallToolResult {
	data, err := json.Marshal(v)
	if err != nil {
		return toolError(
			"internal error: cannot marshal result: %s", err,
		)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}
