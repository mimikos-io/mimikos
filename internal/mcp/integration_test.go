package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_FullLifecycle exercises the complete MCP tool lifecycle:
// start_server → list_endpoints → get_endpoint → stop_server → server_status.
func TestIntegration_FullLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	port := freePort(t)

	// 1. Start server.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port,
	})

	require.False(t, result.IsError,
		"start_server failed: %s", resultText(t, result))

	startData := resultJSON(t, result)
	assert.InDelta(t, float64(port), startData["port"], 0.1)
	assert.Equal(t, "Swagger Petstore", startData["spec_title"])
	assert.InDelta(t, 3.0, startData["operations"], 0.1)

	// 2. List endpoints.
	result = callTool(t, session, "list_endpoints", nil)

	require.False(t, result.IsError,
		"list_endpoints failed: %s", resultText(t, result))

	var endpoints []map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(resultText(t, result)), &endpoints,
	))
	assert.Len(t, endpoints, 3)

	// 3. Get endpoint detail for each endpoint.
	for _, ep := range endpoints {
		method, _ := ep["method"].(string)
		path, _ := ep["path"].(string)

		result = callTool(t, session, "get_endpoint", map[string]any{
			"method": method,
			"path":   path,
		})

		require.False(t, result.IsError,
			"get_endpoint %s %s failed: %s", method, path, resultText(t, result))

		detail := resultJSON(t, result)
		assert.Equal(t, method, detail["method"])
		assert.Equal(t, path, detail["path"])
		assert.NotEmpty(t, detail["behavior"])
		assert.NotNil(t, detail["success_code"])
	}

	// 4. Stop server.
	result = callTool(t, session, "stop_server", nil)

	require.False(t, result.IsError,
		"stop_server failed: %s", resultText(t, result))

	stopData := resultJSON(t, result)
	assert.Equal(t, true, stopData["stopped"])

	// 5. Server status should report stopped.
	result = callTool(t, session, "server_status", nil)

	require.False(t, result.IsError)

	statusData := resultJSON(t, result)
	assert.Equal(t, false, statusData["running"])
}

// TestIntegration_StopAndRestart verifies that after stopping, a new server
// can be started (possibly with different config).
func TestIntegration_StopAndRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// Start deterministic.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     freePort(t),
		"mode":     "deterministic",
	})
	require.False(t, result.IsError)

	data := resultJSON(t, result)
	assert.Equal(t, "deterministic", data["mode"])

	// Stop.
	result = callTool(t, session, "stop_server", nil)
	require.False(t, result.IsError)

	// Restart with stateful mode.
	port2 := freePort(t)

	result = callTool(t, session, "start_server", map[string]any{
		"specPath": petstoreSpecPath(t),
		"port":     port2,
		"mode":     "stateful",
	})
	require.False(t, result.IsError)

	data = resultJSON(t, result)
	assert.Equal(t, "stateful", data["mode"])
	assert.InDelta(t, float64(port2), data["port"], 0.1)
}

// TestIntegration_StatefulLifecycle exercises the stateful CRUD flow:
// start (stateful) → create → list → get → delete → get (404) → reset → stop.
//
// Uses petstore-3.1.yaml because it defines DELETE /pets/{petId} — petstore-3.0
// only has list, create, and fetch.
func TestIntegration_StatefulLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// 1. Start server in stateful mode.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": petstore31SpecPath(t),
		"port":     freePort(t),
		"mode":     "stateful",
	})

	require.False(t, result.IsError,
		"start_server failed: %s", resultText(t, result))

	// 2. Create a pet — only name is required by NewPet schema. The server
	//    generates an id in the 201 response which we extract for subsequent
	//    operations.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"name": "Milo", "tag": "cat"},
	})

	require.False(t, result.IsError,
		"create failed: %s", resultText(t, result))

	createData := resultJSON(t, result)
	assert.InDelta(t, 201.0, createData["status_code"], 0.1)

	// Extract pet ID from the create response body.
	createBody, ok := createData["body"].(map[string]any)
	require.True(t, ok, "create body should be a JSON object")

	petID := fmt.Sprintf("%v", createBody["id"])
	require.NotEmpty(t, petID, "created pet should have an id")

	petPath := "/pets/" + petID

	// 3. List pets — should have 1 resource.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "list",
		"path":   "/pets",
	})

	require.False(t, result.IsError,
		"list failed: %s", resultText(t, result))

	// 4. Get the pet by ID.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "get",
		"path":   petPath,
	})

	require.False(t, result.IsError,
		"get failed: %s", resultText(t, result))

	getData := resultJSON(t, result)
	assert.InDelta(t, 200.0, getData["status_code"], 0.1)

	// 5. Delete the pet.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "delete",
		"path":   petPath,
	})

	require.False(t, result.IsError,
		"delete failed: %s", resultText(t, result))

	deleteData := resultJSON(t, result)
	assert.InDelta(t, 204.0, deleteData["status_code"], 0.1)

	// 6. Get again — should 404.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "get",
		"path":   petPath,
	})

	require.False(t, result.IsError)

	getAfterDelete := resultJSON(t, result)
	assert.InDelta(t, 404.0, getAfterDelete["status_code"], 0.1)

	// 7. Create another pet then reset all state.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/pets",
		"body":   map[string]any{"name": "Luna", "tag": "dog"},
	})
	require.False(t, result.IsError)

	result = callTool(t, session, "manage_state", map[string]any{
		"action": "reset",
	})

	require.False(t, result.IsError,
		"reset failed: %s", resultText(t, result))

	resetData := resultJSON(t, result)
	assert.Equal(t, true, resetData["reset"])

	// 8. Stop server.
	result = callTool(t, session, "stop_server", nil)

	require.False(t, result.IsError,
		"stop_server failed: %s", resultText(t, result))
}

// TestIntegration_StatefulUpdate exercises the manage_state "update" action
// which maps to PUT. Verifies the shallow-merge behavior: updated fields are
// overwritten, untouched fields are preserved.
//
// Uses e2e-status-test.yaml which defines PUT /resources/{id}.
func TestIntegration_StatefulUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)

	// 1. Start server in stateful mode.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": statusTestSpecPath(t),
		"port":     freePort(t),
		"mode":     "stateful",
	})

	require.False(t, result.IsError,
		"start_server failed: %s", resultText(t, result))

	// 2. Create a resource.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "create",
		"path":   "/resources",
		"body":   map[string]any{"name": "original", "description": "keep me"},
	})

	require.False(t, result.IsError,
		"create failed: %s", resultText(t, result))

	createData := resultJSON(t, result)
	assert.InDelta(t, 201.0, createData["status_code"], 0.1)

	// Extract the server-generated ID from the create response.
	createBody, ok := createData["body"].(map[string]any)
	require.True(t, ok, "create body should be a JSON object")

	resourceID := fmt.Sprintf("%v", createBody["id"])
	require.NotEmpty(t, resourceID, "created resource should have an id")

	resourcePath := "/resources/" + resourceID

	// 3. Update — change name, leave description untouched.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "update",
		"path":   resourcePath,
		"body":   map[string]any{"name": "updated"},
	})

	require.False(t, result.IsError,
		"update failed: %s", resultText(t, result))

	updateData := resultJSON(t, result)
	assert.InDelta(t, 200.0, updateData["status_code"], 0.1)

	// 4. Get — verify shallow merge: name changed, description preserved.
	result = callTool(t, session, "manage_state", map[string]any{
		"action": "get",
		"path":   resourcePath,
	})

	require.False(t, result.IsError,
		"get failed: %s", resultText(t, result))

	getBody, ok := resultJSON(t, result)["body"].(map[string]any)
	require.True(t, ok, "get body should be a JSON object")

	assert.Equal(t, "updated", getBody["name"],
		"name should be overwritten by update")
	assert.Equal(t, "keep me", getBody["description"],
		"description should be preserved (shallow merge)")

	// 5. Stop server.
	result = callTool(t, session, "stop_server", nil)

	require.False(t, result.IsError,
		"stop_server failed: %s", resultText(t, result))
}

// statusTestSpecPath returns the path to the e2e-status-test spec, which
// defines explicit 404/422/500 responses (unlike petstore's `default` only).
func statusTestSpecPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(testdataDir(t), "specs", "e2e-status-test.yaml")
}

// TestIntegration_RequestStatusOverride exercises the full request_status flow:
// start → request_status (set 404) → HTTP GET (verify 404) → HTTP GET (verify
// normal 200, override consumed) → get_request_log (verify both captured) → stop.
//
// Uses e2e-status-test.yaml which defines explicit 404 responses — the petstore
// spec only has a `default` error response and doesn't support explicit 404 via
// X-Mimikos-Status.
func TestIntegration_RequestStatusOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("binds a TCP port")
	}

	session, _ := setupMCPSessionWithServer(t)
	port := freePort(t)

	// 1. Start server.
	result := callTool(t, session, "start_server", map[string]any{
		"specPath": statusTestSpecPath(t),
		"port":     port,
	})

	require.False(t, result.IsError,
		"start_server failed: %s", resultText(t, result))

	// 2. Set a 404 override for GET /resources/123.
	result = callTool(t, session, "request_status", map[string]any{
		"method":     "GET",
		"path":       "/resources/123",
		"statusCode": 404,
	})

	require.False(t, result.IsError,
		"request_status failed: %s", resultText(t, result))

	// 3. First HTTP request — should get 404 (override consumed).
	resp := doHTTP(t, port, http.MethodGet, "/resources/123")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp.Body.Close()

	// 4. Second HTTP request — override consumed, normal 200.
	resp = doHTTP(t, port, http.MethodGet, "/resources/123")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp.Body.Close()

	// 5. get_request_log — should have the 2 HTTP requests.
	result = callTool(t, session, "get_request_log", nil)

	require.False(t, result.IsError,
		"get_request_log failed: %s", resultText(t, result))

	var logEntries []map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(resultText(t, result)), &logEntries,
	))
	assert.GreaterOrEqual(t, len(logEntries), 2, "should have at least 2 log entries")

	// Newest first: second request (200), then first request (404).
	assert.InDelta(t, 200.0, logEntries[0]["statusCode"], 0.1)
	assert.InDelta(t, 404.0, logEntries[1]["statusCode"], 0.1)

	// 6. Stop server.
	result = callTool(t, session, "stop_server", nil)

	require.False(t, result.IsError,
		"stop_server failed: %s", resultText(t, result))
}

// doHTTP is a helper that sends an HTTP request to localhost on the given port.
func doHTTP(t *testing.T, port int, method, path string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(
		context.Background(), method,
		fmt.Sprintf("http://localhost:%d%s", port, path), nil,
	)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

// TestIntegration_ErrorMessages verifies that error messages are actionable
// and tell the AI agent exactly what to do next.
func TestIntegration_ErrorMessages(t *testing.T) {
	session := setupMCPSession(t)

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		contains []string
	}{
		{
			name: "list without server",
			tool: "list_endpoints",
			args: nil,
			contains: []string{
				"No Mimikos server is running",
				"start_server",
			},
		},
		{
			name: "get without server",
			tool: "get_endpoint",
			args: map[string]any{"method": "GET", "path": "/pets"},
			contains: []string{
				"No Mimikos server is running",
				"start_server",
			},
		},
		{
			name: "stop without server",
			tool: "stop_server",
			args: nil,
			contains: []string{
				"no server is running",
				"start_server",
			},
		},
		{
			name: "missing spec file",
			tool: "start_server",
			args: map[string]any{
				"specPath": "/nonexistent.yaml",
				"port":     freePort(t),
			},
			contains: []string{
				"Cannot read spec file",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callTool(t, session, tt.tool, tt.args)
			assert.True(t, result.IsError, "should be an error")

			text := resultText(t, result)
			for _, s := range tt.contains {
				assert.Contains(t, text, s,
					"error message should contain %q", s)
			}
		})
	}
}
