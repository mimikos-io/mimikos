package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/model"
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

	// endpointInfo is the JSON shape for endpoint summaries in tool responses.
	endpointInfo struct {
		Method     string `json:"method"`
		Path       string `json:"path"`
		Behavior   string `json:"behavior"`
		Confidence string `json:"confidence"`
	}

	// manageStateArgs holds the input parameters for the manage_state tool.
	manageStateArgs struct {
		Action string         `json:"action"         jsonschema:"Action: list, get, create, update, delete, or reset"`
		Path   string         `json:"path,omitempty" jsonschema:"Resource path (required for all actions except reset)"`
		Body   map[string]any `json:"body,omitempty" jsonschema:"Request body (required for create)"`
	}

	// requestStatusArgs holds the input parameters for the request_status tool.
	requestStatusArgs struct {
		Method     string `json:"method"     jsonschema:"HTTP method (GET, POST, PUT, PATCH, DELETE)"`
		Path       string `json:"path"       jsonschema:"Concrete URL path (e.g. /pets/42, not /pets/{petId})"`
		StatusCode int    `json:"statusCode" jsonschema:"HTTP status code to return on the next request"`
	}

	// getRequestLogArgs holds the input parameters for the get_request_log tool.
	getRequestLogArgs struct {
		Limit int `json:"limit,omitempty" jsonschema:"Max entries to return, newest first (default 20)"`
	}
)

const (
	defaultPort            = 8080
	defaultMaxDepth        = 10
	defaultRequestLogLimit = 20
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
	entries := s.instance.StartupEntries()

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
	entries := s.instance.Endpoints()
	if entries == nil {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	endpoints := make([]endpointInfo, 0, len(entries))
	for _, e := range entries {
		endpoints = append(endpoints, endpointInfo{
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
	method := strings.ToUpper(args.Method)

	entry, ok := s.instance.GetEndpoint(method, args.Path)
	if !ok {
		if !s.instance.IsRunning() {
			return toolError(
				"No Mimikos server is running. Call start_server first.",
			), nil, nil
		}

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

// registerManageState registers the manage_state tool with the MCP server.
func (s *Server) registerManageState() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "manage_state",
		Description: "Manage stateful resources via the running Mimikos " +
			"server (stateful mode only). Actions: list, get, create, update, delete, reset.",
	}, s.handleManageState)
}

// httpMethodForAction returns the HTTP method for a manage_state action.
//
// The "update" action maps to PUT. Mimikos does not distinguish PUT from PATCH
// — the router's handleUpdate does a shallow merge of the request body onto
// the stored resource regardless of HTTP method. PUT is the canonical choice
// because the classifier names the behavior "update", not "put" or "patch".
func httpMethodForAction(action string) (string, bool) {
	switch action {
	case "list", "get":
		return http.MethodGet, true
	case "create":
		return http.MethodPost, true
	case "update":
		return http.MethodPut, true
	case "delete":
		return http.MethodDelete, true
	default:
		return "", false
	}
}

// handleManageState delegates state management to the HTTP handler via
// httptest, reusing all validation, identity extraction, and error handling.
func (s *Server) handleManageState(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args manageStateArgs,
) (*mcp.CallToolResult, any, error) {
	if !s.instance.IsRunning() {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	if s.instance.Mode() != model.ModeStateful {
		return toolError(
			"manage_state requires stateful mode. Current mode: %s. "+
				"Restart with mode \"stateful\".",
			string(s.instance.Mode()),
		), nil, nil
	}

	action := strings.ToLower(args.Action)

	// Handle reset separately — no HTTP equivalent.
	if action == "reset" {
		store := s.instance.Store()
		if store != nil {
			store.Reset()
		}

		return toolJSON(map[string]any{"reset": true}), nil, nil
	}

	// Validate action.
	httpMethod, ok := httpMethodForAction(action)
	if !ok {
		return toolError(
			"Unknown action: %q. Must be one of: list, get, create, update, delete, reset.",
			args.Action,
		), nil, nil
	}

	// Validate path is provided for non-reset actions.
	if args.Path == "" {
		return toolError(
			"\"path\" is required for action %q. Example: /pets or /pets/123.",
			action,
		), nil, nil
	}

	// Validate body for create and update.
	if (action == "create" || action == "update") && len(args.Body) == 0 {
		return toolError(
			"\"body\" is required for action %q. "+
				"Provide the resource fields as a JSON object.",
			action,
		), nil, nil
	}

	// Build the HTTP request.
	var bodyReader *bytes.Reader

	if args.Body != nil {
		bodyBytes, err := json.Marshal(args.Body)
		if err != nil {
			return toolError("cannot marshal body: %s", err), nil, nil
		}

		bodyReader = bytes.NewReader(bodyBytes)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(httpMethod, args.Path, bodyReader)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()

	// Delegate to the router handler.
	handler := s.instance.Handler()
	if handler == nil {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	handler.ServeHTTP(rec, req)

	// Parse the response body as JSON so the agent receives a structured
	// object instead of a double-encoded JSON string.
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		body = rec.Body.String()
	}

	result := map[string]any{
		"status_code": rec.Code,
		"body":        body,
	}

	return toolJSON(result), nil, nil
}

// registerRequestStatus registers the request_status tool with the MCP server.
func (s *Server) registerRequestStatus() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "request_status",
		Description: "Force a specific HTTP status code for the next " +
			"request to an endpoint (one-shot override).",
	}, s.handleRequestStatus)
}

// handleRequestStatus sets a one-shot status code override. The path must be a
// concrete URL path (e.g., /pets/42), not a route pattern — the middleware
// matches overrides against the actual request URL.
func (s *Server) handleRequestStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args requestStatusArgs,
) (*mcp.CallToolResult, any, error) {
	overrides := s.instance.Overrides()
	if overrides == nil {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	method := strings.ToUpper(args.Method)

	overrides.Set(method, args.Path, args.StatusCode)

	result := map[string]any{
		"set":         true,
		"method":      method,
		"path":        args.Path,
		"status_code": args.StatusCode,
	}

	return toolJSON(result), nil, nil
}

// registerGetRequestLog registers the get_request_log tool with the MCP server.
func (s *Server) registerGetRequestLog() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "get_request_log",
		Description: "Get recent HTTP requests and their status codes " +
			"from the running Mimikos server.",
	}, s.handleGetRequestLog)
}

// handleGetRequestLog returns recent entries from the request log.
func (s *Server) handleGetRequestLog(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args getRequestLogArgs,
) (*mcp.CallToolResult, any, error) {
	reqLog := s.instance.ReqLog()
	if reqLog == nil {
		return toolError(
			"No Mimikos server is running. Call start_server first.",
		), nil, nil
	}

	limit := args.Limit
	if limit <= 0 {
		limit = defaultRequestLogLimit
	}

	entries := reqLog.Recent(limit)

	type logEntry struct {
		Method           string   `json:"method"`
		Path             string   `json:"path"`
		StatusCode       int      `json:"statusCode"`
		Timestamp        string   `json:"timestamp"`
		ValidationErrors []string `json:"validationErrors,omitempty"`
	}

	result := make([]logEntry, 0, len(entries))

	for _, e := range entries {
		result = append(result, logEntry{
			Method:           e.Method,
			Path:             e.Path,
			StatusCode:       e.StatusCode,
			Timestamp:        e.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			ValidationErrors: e.ValidationErrors,
		})
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
