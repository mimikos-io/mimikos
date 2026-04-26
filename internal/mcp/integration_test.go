package mcp

import (
	"encoding/json"
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
