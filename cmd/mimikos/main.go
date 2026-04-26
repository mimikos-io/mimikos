// Package main is the entry point for the mimikos CLI.
// Mimikos is a deterministic mock server that generates realistic API
// responses directly from an OpenAPI specification.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	mcpserver "github.com/mimikos-io/mimikos/internal/mcp"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/server"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Build-time variables injected via ldflags. When not set (e.g., go install),
// init resolves version from Go module info embedded by the toolchain.
//
//nolint:gochecknoglobals // ldflags-injected build metadata
var (
	version   = devVersion
	gitCommit = "unknown"
	buildTime = "unknown"

	// ErrInvalidLogLevel is returned when an unrecognized log level string is provided.
	ErrInvalidLogLevel = errors.New("invalid log level")
)

const (
	// shortHashLen is the number of characters used for abbreviated git commit hashes.
	shortHashLen = 7

	// shutdownTimeout is the maximum time to wait for in-flight requests to
	// complete during graceful shutdown.
	shutdownTimeout = 5 * time.Second

	// defaultPort is the default server port.
	defaultPort = 8080

	// defaultMaxDepth is the default maximum recursion depth for data generation.
	defaultMaxDepth = 10

	// defaultMaxResources is the default state store capacity for stateful mode.
	defaultMaxResources = 10_000

	// maxPort is the maximum valid TCP port number.
	maxPort = 65535

	// readHeaderTimeout is the maximum time to read request headers.
	readHeaderTimeout = 10 * time.Second

	// updateCheckTimeout is the maximum time to wait for the GitHub Releases
	// API when checking for a newer version at startup.
	updateCheckTimeout = 2 * time.Second
)

func main() {
	resolveVersionFromBuildInfo()
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run executes the CLI and returns an exit code. Extracted from main for
// testability.
func run(args []string, out *os.File) int {
	if len(args) == 0 {
		printUsage(out)

		return 1
	}

	switch args[0] {
	case "start":
		return runStart(args[1:], out)
	case "mcp":
		return runMCP(args[1:], out)
	case "--help", "-h", "help":
		printUsage(out)

		return 0
	case "--version", "-v", "version":
		output := "🎭 mimikos"

		if version != "0.0.0-dev" {
			output = fmt.Sprintf("%s %s", output, version)
		}

		if gitCommit != "unknown" && buildTime != "unknown" {
			output = fmt.Sprintf("%s (%s, %s)", output, gitCommit, buildTime)
		}

		_, _ = fmt.Fprintf(out, "%s\n", output)

		return 0
	default:
		_, _ = fmt.Fprintf(out, "unknown command: %s\n\n", args[0])
		printUsage(out)

		return 1
	}
}

// startConfig holds parsed and validated "start" subcommand configuration.
type startConfig struct {
	specPath     string
	port         int
	strict       bool
	maxDepth     int
	maxResources int
	level        slog.Level
	mode         model.OperatingMode
}

// parseStartFlags parses and validates CLI flags for the "start" subcommand.
// Returns nil config and prints errors on validation failure.
func parseStartFlags(args []string, out *os.File) *startConfig {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	fs.SetOutput(out)

	port := fs.Int("port", defaultPort, "server port")
	strict := fs.Bool("strict", false, "return 500 if response fails schema validation")
	maxDepth := fs.Int("max-depth", defaultMaxDepth, "max recursion depth for circular schemas")
	maxResources := fs.Int("max-resources", defaultMaxResources, "max stored resources in stateful mode")
	logLevel := fs.String("log-level", "info", "logging verbosity (debug, info, warn, error)")
	modeStr := fs.String("mode", "deterministic", "operating mode (deterministic, stateful)")

	if err := fs.Parse(args); err != nil {
		// flag.ContinueOnError: Parse already printed the error + usage.
		return nil
	}

	if fs.NArg() == 0 {
		_, _ = fmt.Fprintln(out, "error: spec file path is required")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "usage: mimikos start [flags] <spec-path>")

		return nil
	}

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: %s\n", err)

		return nil
	}

	mode, err := model.ParseOperatingMode(*modeStr)
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: %s\n", err)

		return nil
	}

	if *port < 1 || *port > maxPort {
		_, _ = fmt.Fprintf(out, "error: invalid port %d (must be 1-65535)\n", *port)

		return nil
	}

	if *maxDepth < 1 {
		_, _ = fmt.Fprintf(out, "error: --max-depth must be at least 1\n")

		return nil
	}

	if *maxResources < 1 {
		_, _ = fmt.Fprintf(out, "error: --max-resources must be at least 1\n")

		return nil
	}

	return &startConfig{
		specPath:     fs.Arg(0),
		port:         *port,
		strict:       *strict,
		maxDepth:     *maxDepth,
		maxResources: *maxResources,
		level:        level,
		mode:         mode,
	}
}

// runStart implements the "start" subcommand.
func runStart(args []string, out *os.File) int {
	cfg := parseStartFlags(args, out)
	if cfg == nil {
		return 1
	}

	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: cfg.level}))

	// Start version check in background (non-blocking).
	updateCh := startUpdateCheck()

	// Read spec file.
	specBytes, err := os.ReadFile(cfg.specPath)
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: cannot read spec file: %s\n", err)

		return 1
	}

	// Build server.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler, result, err := server.Build(ctx, specBytes, server.Config{
		Strict:       cfg.strict,
		MaxDepth:     cfg.maxDepth,
		MaxResources: cfg.maxResources,
		Logger:       logger,
		Mode:         cfg.mode,
	})
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: %s\n", err)

		return 1
	}

	// Bind listener before printing banner so the port is confirmed available.
	addr := fmt.Sprintf(":%d", cfg.port)

	var listenerConfig net.ListenConfig

	listener, err := listenerConfig.Listen(ctx, "tcp", addr)
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: cannot listen on %s: %s\n", addr, err)

		return 1
	}

	// Blank line separates slog warnings from the startup banner.
	if result.FailedEntries > 0 || result.DegradedSchemas > 0 {
		_, _ = fmt.Fprintln(out)
	}

	// Print startup summary after successful bind.
	printStartupSummary(out, result, cfg.port, cfg.strict)

	// Print update notification if the check completed during startup.
	printUpdateNotification(out, updateCh)

	// Start HTTP server on the pre-bound listener.
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Run server in background.
	errCh := make(chan error, 1)

	go func() {
		if srvErr := srv.Serve(listener); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			errCh <- srvErr
		}

		close(errCh)
	}()

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		stop() // Restore default signal handling.

		_, _ = fmt.Fprintln(out, "\nShutting down...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if shutdownErr := srv.Shutdown(shutdownCtx); shutdownErr != nil {
			_, _ = fmt.Fprintf(out, "error: shutdown timed out: %s\n", shutdownErr)

			return 1
		}

		_, _ = fmt.Fprintln(out, "Server stopped.")

		return 0
	case srvErr := <-errCh:
		if srvErr != nil {
			_, _ = fmt.Fprintf(out, "error: server failed: %s\n", srvErr)

			return 1
		}

		return 0
	}
}

// runMCP implements the "mcp" subcommand. It starts an MCP server over stdio
// that exposes Mimikos tools to AI agents. Diagnostics go to stderr only —
// stdout is reserved for the MCP transport.
func runMCP(args []string, out *os.File) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(out)

	logLevel := fs.String("log-level", "info", "logging verbosity (debug, info, warn, error)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	level, err := parseLogLevel(*logLevel)
	if err != nil {
		_, _ = fmt.Fprintf(out, "error: %s\n", err)

		return 1
	}

	// MCP diagnostics go to stderr, never stdout (stdout is the transport).
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: level}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := mcpserver.NewServer(version, logger)

	if err := srv.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
		_, _ = fmt.Fprintf(out, "error: MCP server failed: %s\n", err)

		return 1
	}

	return 0
}

// printStartupSummary writes the human-friendly startup banner.
func printStartupSummary(out *os.File, result *server.StartupResult, port int, strict bool) {
	_, _ = fmt.Fprintf(out, "🎭 mimikos %s\n", version)
	_, _ = fmt.Fprintf(out, "Spec: %s (OpenAPI %s)\n", result.SpecTitle, result.SpecVersion)
	_, _ = fmt.Fprintf(out, "Operations: %d endpoints classified\n", result.Operations)

	if result.FailedEntries > 0 {
		_, _ = fmt.Fprintf(out, "⚠ %d endpoint(s) failed to register:\n", result.FailedEntries)

		for _, p := range result.FailedPaths {
			_, _ = fmt.Fprintf(out, "    %s\n", p)
		}
	}

	if result.DegradedSchemas > 0 {
		_, _ = fmt.Fprintf(out, "⚠ %d endpoint(s) have degraded schemas:\n", result.DegradedSchemas)

		for _, p := range result.DegradedPaths {
			_, _ = fmt.Fprintf(out, "    %s\n", p)
		}
	}

	_, _ = fmt.Fprintln(out)

	if len(result.Entries) > 0 {
		// Compute column widths from entries and headers.
		mw, pw, bw := len("METHOD"), len("PATH"), len("BEHAVIOR")
		for _, e := range result.Entries {
			if len(e.Method) > mw {
				mw = len(e.Method)
			}

			if len(e.PathPattern) > pw {
				pw = len(e.PathPattern)
			}

			if len(e.BehaviorType) > bw {
				bw = len(e.BehaviorType)
			}
		}

		rowFmt := fmt.Sprintf("  %%-%ds %%-%ds → %%-%ds %%s\n", mw, pw, bw)
		colFmt := fmt.Sprintf("  %%-%ds %%-%ds %%-%ds %%s\n", mw, pw, bw)

		_, _ = fmt.Fprintf(out, colFmt, "METHOD", "PATH", "BEHAVIOR", "CONFIDENCE")

		for _, e := range result.Entries {
			_, _ = fmt.Fprintf(out, rowFmt, e.Method, e.PathPattern, e.BehaviorType, e.Confidence)
		}

		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintf(out, "Listening on :%d (%s mode, strict=%t)\n", port, result.Mode, strict)
}

// startUpdateCheck launches a background goroutine that checks the GitHub
// Releases API for a newer mimikos version. Returns a buffered channel that
// receives the result (or nil on failure/skip). The goroutine uses an
// independent context so it completes even if the main context is cancelled.
func startUpdateCheck() <-chan *updateResult {
	ch := make(chan *updateResult, 1)

	go func() {
		checkCtx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		ch <- checkLatestVersion(checkCtx, version, "")
	}()

	return ch
}

// printUpdateNotification does a non-blocking read on the update channel
// and prints a notification if a newer version is available. If the check
// is still in flight, it silently moves on — never blocks startup.
func printUpdateNotification(out *os.File, ch <-chan *updateResult) {
	select {
	case r := <-ch:
		if r != nil && r.UpdateAvailable {
			_, _ = fmt.Fprint(out, formatUpdateNotification(r.CurrentVersion, r.LatestVersion))
		}
	default:
		// Check still in flight — don't block startup.
	}
}

// printUsage writes the CLI usage message.
func printUsage(out *os.File) {
	_, _ = fmt.Fprintln(out, "🎭 mimikos — deterministic mock server from OpenAPI specs")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Usage:")
	_, _ = fmt.Fprintln(out, "  mimikos start [flags] <spec-path>")
	_, _ = fmt.Fprintln(out, "  mimikos mcp [flags]")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Commands:")
	_, _ = fmt.Fprintln(out, "  start       Start the mock server")
	_, _ = fmt.Fprintln(out, "  mcp         Start as MCP server (for AI agent integration)")
	_, _ = fmt.Fprintln(out, "  version     Show version information")
	_, _ = fmt.Fprintln(out, "  help        Show this help message")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Flags (for start):")
	_, _ = fmt.Fprintln(out, "  --port           Server port (default: 8080)")
	_, _ = fmt.Fprintln(out, "  --mode           Operating mode: deterministic, stateful (default: deterministic)")
	_, _ = fmt.Fprintln(out, "  --strict         Return 500 if response fails schema validation (default: false)")
	_, _ = fmt.Fprintln(out, "  --max-depth      Max recursion depth for circular schemas (default: 3)")
	_, _ = fmt.Fprintln(out, "  --max-resources  Max stored resources in stateful mode (default: 10000)")
	_, _ = fmt.Fprintln(out, "  --log-level      Logging verbosity: debug, info, warn, error (default: info)")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Aliases:")
	_, _ = fmt.Fprintln(out, "  mimik        Short alias for mimikos")
}

// parseLogLevel converts a log level string to slog.Level.
func parseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%w: %q (must be debug, info, warn, or error)", ErrInvalidLogLevel, s)
	}
}

// resolveVersionFromBuildInfo populates version, gitCommit, and buildTime from
// Go module info embedded by the toolchain. This covers the go install path
// where ldflags are not injected.
func resolveVersionFromBuildInfo() {
	if version != "0.0.0-dev" {
		return // ldflags were set (GoReleaser build), nothing to resolve.
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= shortHashLen {
				gitCommit = s.Value[:shortHashLen]
			}
		case "vcs.time":
			buildTime = s.Value
		}
	}
}
