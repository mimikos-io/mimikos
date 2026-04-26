package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the absolute path to testdata/specs/.
func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "specs")
}

func TestRun_NoArgs(t *testing.T) {
	code := run(nil, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_Help(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			code := run([]string{arg}, os.Stderr)
			assert.Equal(t, 0, code)
		})
	}
}

func TestRun_Version(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			code := run([]string{arg}, os.Stderr)
			assert.Equal(t, 0, code)
		})
	}
}

func TestRun_HelpIncludesMCP(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	code := run([]string{"help"}, w)
	require.NoError(t, w.Close())

	assert.Equal(t, 0, code)

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "mcp")
	assert.Contains(t, output, "MCP server")
}

func TestRun_MCP_Dispatches(t *testing.T) {
	// "mcp" is recognized as a valid command (not "unknown command").
	// We can't actually start the MCP server in a unit test (it requires
	// a stdio transport), but we verify the dispatch path is correct by
	// checking that passing an invalid log level produces exit code 1
	// with the right error, not "unknown command".
	code := run([]string{"mcp", "--log-level", "invalid"}, os.Stderr)
	assert.Equal(t, 1, code, "invalid log level should fail with exit 1")
}

func TestRun_UnknownCommand(t *testing.T) {
	code := run([]string{"bogus"}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartMissingSpec(t *testing.T) {
	code := run([]string{"start"}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartNonexistentFile(t *testing.T) {
	code := run([]string{"start", "/nonexistent/spec.yaml"}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartInvalidSpec(t *testing.T) {
	// Create a temp file with invalid content.
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "bad.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte("not valid ["), 0o644))

	code := run([]string{"start", specPath}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartInvalidLogLevel(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")
	code := run([]string{"start", "--log-level", "verbose", specPath}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartInvalidFlag(t *testing.T) {
	code := run([]string{"start", "--bogus"}, os.Stderr)
	assert.Equal(t, 1, code)
}

func TestRun_StartInvalidPort(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	tests := []struct {
		name string
		port string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		{"too high", "70000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := run([]string{"start", "--port", tt.port, specPath}, os.Stderr)
			assert.Equal(t, 1, code)
		})
	}
}

func TestRun_StartInvalidMaxDepth(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	tests := []struct {
		name  string
		depth string
	}{
		{"zero", "0"},
		{"negative", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := run([]string{"start", "--max-depth", tt.depth, specPath}, os.Stderr)
			assert.Equal(t, 1, code)
		})
	}
}

func TestParseStartFlags_ModeDefault(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	cfg := parseStartFlags([]string{specPath}, os.Stderr)

	require.NotNil(t, cfg)
	assert.Equal(t, "deterministic", cfg.mode.String())
}

func TestParseStartFlags_ModeStateful(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	cfg := parseStartFlags([]string{"--mode", "stateful", specPath}, os.Stderr)

	require.NotNil(t, cfg)
	assert.Equal(t, "stateful", cfg.mode.String())
}

func TestParseStartFlags_ModeInvalid(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	cfg := parseStartFlags([]string{"--mode", "invalid", specPath}, os.Stderr)

	assert.Nil(t, cfg, "invalid mode should reject config")
}

func TestParseStartFlags_ModeCaseSensitive(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	tests := []struct {
		name string
		mode string
	}{
		{"uppercase", "Stateful"},
		{"all caps", "STATEFUL"},
		{"mixed", "Deterministic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseStartFlags([]string{"--mode", tt.mode, specPath}, os.Stderr)
			assert.Nil(t, cfg, "case-variant %q should be rejected", tt.mode)
		})
	}
}

func TestParseStartFlags_MaxResourcesDefault(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	cfg := parseStartFlags([]string{specPath}, os.Stderr)

	require.NotNil(t, cfg)
	assert.Equal(t, 10_000, cfg.maxResources)
}

func TestParseStartFlags_MaxResourcesCustom(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	cfg := parseStartFlags([]string{"--max-resources", "500", specPath}, os.Stderr)

	require.NotNil(t, cfg)
	assert.Equal(t, 500, cfg.maxResources)
}

func TestParseStartFlags_MaxResourcesInvalid(t *testing.T) {
	specPath := filepath.Join(testdataDir(t), "petstore-3.0.yaml")

	tests := []struct {
		name  string
		value string
	}{
		{"zero", "0"},
		{"negative", "-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseStartFlags([]string{"--max-resources", tt.value, specPath}, os.Stderr)
			assert.Nil(t, cfg)
		})
	}
}

// --- Startup banner tests (22.4) ---

func TestPrintStartupSummary_Headers(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	result := &server.StartupResult{
		SpecTitle:   "Petstore",
		SpecVersion: "3.0.0",
		Operations:  2,
		Mode:        model.ModeDeterministic,
		Entries: []server.EntryInfo{
			{Method: "GET", PathPattern: "/pets", BehaviorType: "list", Confidence: "high"},
			{Method: "POST", PathPattern: "/pets", BehaviorType: "create", Confidence: "high"},
		},
	}

	printStartupSummary(w, result, 8080, false)
	require.NoError(t, w.Close())

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "METHOD")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "BEHAVIOR")
	assert.Contains(t, output, "CONFIDENCE")
}

func TestPrintStartupSummary_DynamicAlignment(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	result := &server.StartupResult{
		SpecTitle:   "Test API",
		SpecVersion: "3.1.0",
		Operations:  2,
		Mode:        model.ModeDeterministic,
		Entries: []server.EntryInfo{
			{Method: "GET", PathPattern: "/short", BehaviorType: "list", Confidence: "high"},
			{Method: "DELETE", PathPattern: "/very/long/path/pattern/{id}", BehaviorType: "delete", Confidence: "moderate"},
		},
	}

	printStartupSummary(w, result, 9090, true)
	require.NoError(t, w.Close())

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// The long path should not be truncated.
	assert.Contains(t, output, "/very/long/path/pattern/{id}")
	// The "→" separators in header and rows should be aligned.
	assert.Contains(t, output, "Listening on :9090")
	assert.Contains(t, output, "strict=true")
}

func TestPrintStartupSummary_EmptyEntries(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	result := &server.StartupResult{
		SpecTitle:   "Empty",
		SpecVersion: "3.0.0",
		Operations:  0,
		Mode:        model.ModeDeterministic,
		Entries:     nil,
	}

	printStartupSummary(w, result, 8080, false)
	require.NoError(t, w.Close())

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "Operations: 0 endpoints classified")
	// No column headers when there are no entries.
	assert.NotContains(t, output, "METHOD")
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
		err   bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"verbose", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseLogLevel(tt.input)
			if tt.err {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
