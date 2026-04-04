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
	"syscall"
	"time"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/server"
)

// Build-time variables injected via ldflags.
//
//nolint:gochecknoglobals // ldflags-injected build metadata
var (
	version   = "0.0.0-dev"
	gitCommit = "unknown"
	buildTime = "unknown"
)

// ErrInvalidLogLevel is returned when an unrecognized log level string is provided.
var ErrInvalidLogLevel = errors.New("invalid log level")

// shutdownTimeout is the maximum time to wait for in-flight requests to
// complete during graceful shutdown.
const shutdownTimeout = 5 * time.Second

// defaultPort is the default server port.
const defaultPort = 8080

// defaultMaxDepth is the default maximum recursion depth for data generation.
const defaultMaxDepth = 3

// defaultMaxResources is the default state store capacity for stateful mode.
const defaultMaxResources = 10_000

// maxPort is the maximum valid TCP port number.
const maxPort = 65535

// readHeaderTimeout is the maximum time to read request headers.
const readHeaderTimeout = 10 * time.Second

func main() {
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
	case "--help", "-h", "help":
		printUsage(out)

		return 0
	case "--version", "-v", "version":
		_, _ = fmt.Fprintf(out, "🎭 mimikos %s (%s, %s)\n", version, gitCommit, buildTime)

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

	// Print startup summary after successful bind.
	printStartupSummary(out, result, cfg.port, cfg.strict)

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

// printStartupSummary writes the human-friendly startup banner.
func printStartupSummary(out *os.File, result *server.StartupResult, port int, strict bool) {
	_, _ = fmt.Fprintf(out, "🎭 mimikos %s\n", version)
	_, _ = fmt.Fprintf(out, "Spec: %s (OpenAPI %s)\n", result.SpecTitle, result.SpecVersion)
	_, _ = fmt.Fprintf(out, "Operations: %d endpoints classified\n", result.Operations)
	_, _ = fmt.Fprintln(out)

	for _, e := range result.Entries {
		_, _ = fmt.Fprintf(out, "  %-7s %-30s → %-10s %s\n",
			e.Method, e.PathPattern, e.BehaviorType, e.Confidence)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Listening on :%d (%s mode, strict=%t)\n", port, result.Mode, strict)
}

// printUsage writes the CLI usage message.
func printUsage(out *os.File) {
	_, _ = fmt.Fprintln(out, "🎭 mimikos — deterministic mock server from OpenAPI specs")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Usage:")
	_, _ = fmt.Fprintln(out, "  mimikos start [flags] <spec-path>")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Commands:")
	_, _ = fmt.Fprintln(out, "  start       Start the mock server")
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
