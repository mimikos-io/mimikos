package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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
