package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

// petstoreSpecPath returns the absolute path to the petstore-3.0.yaml spec.
func petstoreSpecPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(testdataDir(t), "specs", "petstore-3.0.yaml")
}

// petstore31SpecPath returns the absolute path to the petstore-3.1.yaml spec,
// which includes DELETE and PATCH operations not present in petstore-3.0.
func petstore31SpecPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(testdataDir(t), "specs", "petstore-3.1.yaml")
}

// callTool is a helper that invokes a tool on a connected MCP server via
// in-memory transport. Returns the parsed result.
func callTool(
	t *testing.T,
	session *mcp.ClientSession,
	name string,
	args map[string]any,
) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: mustMarshal(t, args),
	})
	require.NoError(t, err, "CallTool %q should not return protocol error", name)

	return result
}

// resultJSON extracts and parses the JSON text from a CallToolResult.
func resultJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()

	require.NotEmpty(t, result.Content, "result should have content")

	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "first content should be TextContent, got %T", result.Content[0])

	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &parsed))

	return parsed
}

// resultText extracts the text from a CallToolResult.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	require.NotEmpty(t, result.Content, "result should have content")

	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "first content should be TextContent")

	return tc.Text
}

// mustMarshal marshals v to json.RawMessage.
func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}

// setupMCPSession creates a Server with in-memory transport and returns the
// client session. Registers cleanup to close the session.
func setupMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := NewServer("0.0.0-test", nil)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Connect server.
	serverSession, err := srv.mcpServer.Connect(
		context.Background(), serverTransport, nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		serverSession.Close()
	})

	// Connect client.
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)

	clientSession, err := client.Connect(
		context.Background(), clientTransport, nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()
	})

	return clientSession
}

// setupMCPSessionWithServer creates a session and a Server, returning both so
// tests can access the instance for cleanup or inspection.
func setupMCPSessionWithServer(t *testing.T) (*mcp.ClientSession, *Server) {
	t.Helper()

	srv := NewServer("0.0.0-test", nil)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	serverSession, err := srv.mcpServer.Connect(
		context.Background(), serverTransport, nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		serverSession.Close()
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil)

	clientSession, err := client.Connect(
		context.Background(), clientTransport, nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		clientSession.Close()
		serverSession.Wait()

		// Ensure mock server is stopped after test.
		if srv.instance.IsRunning() {
			_ = srv.instance.Stop(context.Background())
		}
	})

	return clientSession, srv
}

// --- Tool registration ---

func TestToolRegistration_ExactlyEightTools(t *testing.T) {
	session := setupMCPSession(t)

	result, err := session.ListTools(
		context.Background(), &mcp.ListToolsParams{},
	)
	require.NoError(t, err)

	wantNames := []string{
		"start_server",
		"stop_server",
		"server_status",
		"list_endpoints",
		"get_endpoint",
		"manage_state",
		"request_status",
		"get_request_log",
	}

	gotNames := make([]string, len(result.Tools))
	for i, tool := range result.Tools {
		gotNames[i] = tool.Name
	}

	assert.Len(t, result.Tools, len(wantNames),
		"expected exactly %d tools, got %d: %v",
		len(wantNames), len(result.Tools), gotNames)

	for _, name := range wantNames {
		found := false

		for _, tool := range result.Tools {
			if tool.Name == name {
				found = true

				break
			}
		}

		assert.True(t, found, "missing tool %q in %v", name, gotNames)
	}
}

// --- start_server tests ---

func TestStartServer_ValidSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, srv := setupMCPSessionWithServer(t)
	port := freePort(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})

	assert.False(t, result.IsError,
		"start_server should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.InDelta(t, float64(port), data["port"], 0.1)
	assert.Equal(t, "Swagger Petstore", data["spec_title"])
	assert.Equal(t, "3.0.0", data["spec_version"])
	assert.InDelta(t, 3.0, data["operations"], 0.1)
	assert.Equal(t, "deterministic", data["mode"])

	endpoints, ok := data["endpoints"].([]any)
	require.True(t, ok, "endpoints should be an array")
	assert.Len(t, endpoints, 3)

	// Verify server is actually running.
	assert.True(t, srv.instance.IsRunning())
}

func TestStartServer_MissingSpecFile(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": "/nonexistent/path/spec.yaml",
		"port":     freePort(t),
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "Cannot read spec file")
}

func TestStartServer_InvalidSpec(t *testing.T) {
	// Write a temporary file with invalid content.
	tmpFile := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte("not: valid: openapi"), 0o644))

	session := setupMCPSession(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": tmpFile,
		"port":     freePort(t),
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "cannot start server")
}

func TestStartServer_AlreadyRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// First start.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError, "first start should succeed")

	// Second start.
	result = callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "already running")
	assert.Contains(t, resultText(t, result), "call stop_server first")
}

func TestStartServer_OptionalParams(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, srv := setupMCPSessionWithServer(t)
	port := freePort(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
		"mode":     "stateful",
		"strict":   true,
		"maxDepth": 5,
	})

	assert.False(t, result.IsError,
		"start_server with optional params should succeed: %s",
		resultText(t, result))

	data := resultJSON(t, result)
	assert.Equal(t, "stateful", data["mode"])

	status := srv.instance.Status()
	assert.Equal(t, "stateful", string(status.Mode))
}

func TestStartServer_InvalidMode(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
		"mode":     "bogus",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "invalid mode")
}

func TestStartServer_PortUnavailable(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	// Occupy a port.
	var lc net.ListenConfig

	listener, err := lc.Listen(context.Background(), "tcp", ":0")
	require.NoError(t, err)

	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	port := tcpAddr.Port
	session := setupMCPSession(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result),
		fmt.Sprintf("port %d is not available", port))
}

// --- stop_server tests ---

func TestStopServer_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, srv := setupMCPSessionWithServer(t)

	// Start first.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Stop.
	result = callTool(t, session, "stop_server", nil)

	assert.False(t, result.IsError,
		"stop_server should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.Equal(t, true, data["stopped"])
	assert.Equal(t, "Swagger Petstore", data["was_spec"])
	assert.False(t, srv.instance.IsRunning())
}

func TestStopServer_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "stop_server", nil)

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "no server is running")
	assert.Contains(t, resultText(t, result), "call start_server first")
}

// --- server_status tests ---

func TestServerStatus_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	port := freePort(t)

	// Start.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})
	require.False(t, result.IsError)

	// Status.
	result = callTool(t, session, "server_status", nil)

	assert.False(t, result.IsError)

	data := resultJSON(t, result)
	assert.Equal(t, true, data["running"])
	assert.InDelta(t, float64(port), data["port"], 0.1)
	assert.Equal(t, "deterministic", data["mode"])
	assert.Equal(t, "Swagger Petstore", data["spec_title"])
	assert.Equal(t, "3.0.0", data["spec_version"])
	assert.InDelta(t, 3.0, data["operations"], 0.1)
	assert.NotNil(t, data["uptime_seconds"])
}

func TestServerStatus_Stopped(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "server_status", nil)

	assert.False(t, result.IsError)

	data := resultJSON(t, result)
	assert.Equal(t, false, data["running"])
}

// --- list_endpoints tests ---

func TestListEndpoints_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// Start server.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// List endpoints.
	result = callTool(t, session, "list_endpoints", nil)

	assert.False(t, result.IsError,
		"list_endpoints should succeed: %s", resultText(t, result))

	// Parse as array.
	var endpoints []map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(resultText(t, result)), &endpoints,
	))
	assert.Len(t, endpoints, 3)

	// Build a lookup by method+path.
	lookup := make(map[string]map[string]any)

	for _, ep := range endpoints {
		key := fmt.Sprintf("%s %s", ep["method"], ep["path"])
		lookup[key] = ep
	}

	// Verify expected endpoints.
	listEp, ok := lookup["GET /pets"]
	require.True(t, ok, "should have GET /pets")
	assert.Equal(t, "list", listEp["behavior"])
	assert.NotEmpty(t, listEp["confidence"])

	createEp, ok := lookup["POST /pets"]
	require.True(t, ok, "should have POST /pets")
	assert.Equal(t, "create", createEp["behavior"])

	fetchEp, ok := lookup["GET /pets/{petId}"]
	require.True(t, ok, "should have GET /pets/{petId}")
	assert.Equal(t, "fetch", fetchEp["behavior"])
}

func TestListEndpoints_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "list_endpoints", nil)

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No Mimikos server is running")
	assert.Contains(t, resultText(t, result), "start_server")
}

// --- get_endpoint tests ---

func TestGetEndpoint_Known(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// Start server.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Get endpoint detail.
	result = callTool(t, session, "get_endpoint", map[string]any{
		"method": "GET",
		"path":   "/pets",
	})

	assert.False(t, result.IsError,
		"get_endpoint should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.Equal(t, "GET", data["method"])
	assert.Equal(t, "/pets", data["path"])
	assert.Equal(t, "list", data["behavior"])
	assert.NotEmpty(t, data["confidence"])
	assert.NotNil(t, data["success_code"])
	assert.NotNil(t, data["has_request_schema"])
	assert.NotNil(t, data["has_response_schema"])
	assert.NotNil(t, data["degraded"])
}

func TestGetEndpoint_CaseInsensitiveMethod(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Use lowercase method.
	result = callTool(t, session, "get_endpoint", map[string]any{
		"method": "get",
		"path":   "/pets",
	})

	assert.False(t, result.IsError, "should handle lowercase method")

	data := resultJSON(t, result)
	assert.Equal(t, "GET", data["method"])
}

func TestGetEndpoint_Unknown(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	result = callTool(t, session, "get_endpoint", map[string]any{
		"method": "DELETE",
		"path":   "/nonexistent",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No endpoint found")
	assert.Contains(t, resultText(t, result), "DELETE /nonexistent")
	assert.Contains(t, resultText(t, result), "list_endpoints")
}

func TestGetEndpoint_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "get_endpoint", map[string]any{
		"method": "GET",
		"path":   "/pets",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No Mimikos server is running")
	assert.Contains(t, resultText(t, result), "start_server")
}

// --- manage_state tests ---

// startStatefulServer is a helper that starts the petstore in stateful mode.
func startStatefulServer(
	t *testing.T,
	session *mcp.ClientSession,
) {
	t.Helper()

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
		"mode":     "stateful",
	})
	require.False(t, result.IsError,
		"start_server (stateful) should succeed: %s", resultText(t, result))
}

func TestManageState_CreateAndList(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	// Create a pet (Pet schema requires id + name).
	result := callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"id": 1, "name": "Buddy", "tag": "dog"},
	})

	assert.False(t, result.IsError,
		"create should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.InDelta(t, 201.0, data["status_code"], 0.1)

	// List pets.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "list",
		"path":   "/pets",
	})

	assert.False(t, result.IsError,
		"list should succeed: %s", resultText(t, result))
}

func TestManageState_GetResource(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	// Create a pet first (Pet schema requires id + name).
	result := callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"id": 42, "name": "Rex", "tag": "dog"},
	})
	require.False(t, result.IsError,
		"create should succeed: %s", resultText(t, result))

	// Get the resource by ID — petstore uses /pets/{petId}.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "get",
		"path":   "/pets/42",
	})

	assert.False(t, result.IsError,
		"get should succeed: %s", resultText(t, result))

	getData := resultJSON(t, result)
	assert.InDelta(t, 200.0, getData["status_code"], 0.1)
}

func TestManageState_DeleteResource(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	// Create a pet (Pet schema requires id + name).
	result := callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"id": 99, "name": "Luna", "tag": "cat"},
	})
	require.False(t, result.IsError,
		"create should succeed: %s", resultText(t, result))

	// Delete it.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "delete",
		"path":   "/pets/99",
	})

	assert.False(t, result.IsError,
		"delete should succeed: %s", resultText(t, result))
}

func TestManageState_Reset(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	// Create a pet (Pet schema requires id + name).
	result := callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"id": 7, "name": "Fido", "tag": "dog"},
	})
	require.False(t, result.IsError)

	// Reset.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "reset",
	})

	assert.False(t, result.IsError,
		"reset should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.Equal(t, true, data["reset"])
}

func TestManageState_MissingPath(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	result := callTool(t, session, "manage_state", map[string]any{
		"action": "list",
		// no path
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "path")
}

func TestManageState_MissingBodyForCreate(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	result := callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		// no body
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "body")
}

func TestManageState_DeterministicModeError(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// Start in deterministic mode (default).
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	result = callTool(t, session, "manage_state", map[string]any{
		"action": "list",
		"path":   "/pets",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "stateful")
}

func TestManageState_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "manage_state", map[string]any{
		"action": "list",
		"path":   "/pets",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No Mimikos server is running")
}

func TestManageState_InvalidAction(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	result := callTool(t, session, "manage_state", map[string]any{
		"action": "purge",
		"path":   "/pets",
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "Unknown action")
}

// --- request_status tests ---

func TestRequestStatus_SetsOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Use concrete path, not pattern.
	result = callTool(t, session, "request_status", map[string]any{
		"method":     "GET",
		"path":       "/pets",
		"statusCode": 500,
	})

	assert.False(t, result.IsError,
		"request_status should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.Equal(t, true, data["set"])
}

func TestRequestStatus_AcceptsAnyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Override for a path that may not match any endpoint — should still
	// succeed. The override simply won't be consumed if no request matches.
	result = callTool(t, session, "request_status", map[string]any{
		"method":     "DELETE",
		"path":       "/nonexistent",
		"statusCode": 404,
	})

	assert.False(t, result.IsError,
		"request_status should accept any path: %s", resultText(t, result))
}

func TestRequestStatus_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "request_status", map[string]any{
		"method":     "GET",
		"path":       "/pets",
		"statusCode": 500,
	})

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No Mimikos server is running")
}

// --- get_request_log tests ---

func TestGetRequestLog_ReturnsEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	port := freePort(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})
	require.False(t, result.IsError)

	// Make some HTTP requests to populate the log.
	for _, path := range []string{"/pets", "/pets", "/pets/123"} {
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet,
			fmt.Sprintf("http://localhost:%d%s", port, path), nil,
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		resp.Body.Close()
	}

	// Get request log.
	result = callTool(t, session, "get_request_log", nil)

	assert.False(t, result.IsError,
		"get_request_log should succeed: %s", resultText(t, result))

	// Parse as array.
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(resultText(t, result)), &entries,
	))
	assert.Len(t, entries, 3)

	// Newest first.
	assert.Equal(t, "/pets/123", entries[0]["path"])
}

func TestGetRequestLog_RespectsLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	port := freePort(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})
	require.False(t, result.IsError)

	// Make 3 requests.
	for range 3 {
		req, err := http.NewRequestWithContext(
			context.Background(), http.MethodGet,
			fmt.Sprintf("http://localhost:%d/pets", port), nil,
		)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		resp.Body.Close()
	}

	// Get only 1.
	result = callTool(t, session, "get_request_log", map[string]any{
		"limit": 1,
	})

	assert.False(t, result.IsError)

	var entries []map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(resultText(t, result)), &entries,
	))
	assert.Len(t, entries, 1)
}

func TestGetRequestLog_NotRunning(t *testing.T) {
	session := setupMCPSession(t)

	result := callTool(t, session, "get_request_log", nil)

	assert.True(t, result.IsError)
	assert.Contains(t, resultText(t, result), "No Mimikos server is running")
}

// --- Edge case tests (Task 50.1) ---

// noOpsSpecPath returns the path to a valid OpenAPI spec with zero operations.
func noOpsSpecPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(testdataDir(t), "specs", "e2e-no-operations.yaml")
}

func TestStartServer_EmptySpecFile(t *testing.T) {
	// An empty (0-byte) spec file should return an actionable error.
	emptyFile := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0o644))

	session := setupMCPSession(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": emptyFile,
		"port":     freePort(t),
	})

	assert.True(t, result.IsError, "empty spec file should be an error")

	text := resultText(t, result)
	assert.Contains(t, text, "cannot start server",
		"error should explain what happened")
}

func TestStartServer_ZeroOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	// A valid OpenAPI spec with paths: {} should start successfully but
	// report 0 operations. This is not an error — it's a valid (if useless)
	// spec.
	session, srv := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": noOpsSpecPath(t),
		"port":     freePort(t),
	})

	assert.False(t, result.IsError,
		"zero-ops spec should succeed: %s", resultText(t, result))

	data := resultJSON(t, result)
	assert.InDelta(t, 0.0, data["operations"], 0.1,
		"should report 0 operations")
	assert.Equal(t, "No Operations API", data["spec_title"])

	endpoints, ok := data["endpoints"].([]any)
	require.True(t, ok, "endpoints should be an array")
	assert.Empty(t, endpoints, "should have no endpoints")

	assert.True(t, srv.instance.IsRunning(),
		"server should still be running")
}

func TestManageState_UpdateMissingBody(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	startStatefulServer(t, session)

	result := callTool(t, session, "manage_state", map[string]any{
		"action": "update",
		"path":   "/pets/42",
		// no body — required for update
	})

	assert.True(t, result.IsError,
		"update without body should be an error")

	text := resultText(t, result)
	assert.Contains(t, text, "body",
		"error should mention missing body")
	assert.Contains(t, text, "update",
		"error should mention the action")
}

func TestGetEndpoint_ValidMethodWrongPathFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
	})
	require.False(t, result.IsError)

	// Valid method (GET exists in spec) but a path that doesn't match any
	// route pattern. The error should guide the user to list_endpoints.
	result = callTool(t, session, "get_endpoint", map[string]any{
		"method": "GET",
		"path":   "pets",
	})

	assert.True(t, result.IsError)

	text := resultText(t, result)
	assert.Contains(t, text, "No endpoint found",
		"error should say no endpoint found")
	assert.Contains(t, text, "list_endpoints",
		"error should guide user to list_endpoints")
}
