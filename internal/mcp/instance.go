package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/server"
	"github.com/mimikos-io/mimikos/internal/state"
)

const (
	// readHeaderTimeout is the maximum time to read request headers.
	readHeaderTimeout = 10 * time.Second

	// shutdownTimeout is the maximum time for graceful shutdown.
	shutdownTimeout = 5 * time.Second
)

// timeNow returns the current time. Replaced in tests for deterministic uptime.
//
//nolint:gochecknoglobals // test seam
var timeNow = time.Now

// Sentinel errors for instance lifecycle operations.
var (
	// ErrAlreadyRunning is returned when Start is called on a running instance.
	ErrAlreadyRunning = errors.New("server is already running")

	// ErrNotRunning is returned when Stop or query operations are called on a stopped instance.
	ErrNotRunning = errors.New("no server is running")

	// ErrInvalidMode is returned when an unrecognized operating mode is provided.
	ErrInvalidMode = errors.New("invalid mode")
)

// StartConfig holds the configuration for starting a Mimikos mock server.
type StartConfig struct {
	// SpecPath is the filesystem path to the OpenAPI spec file.
	SpecPath string

	// SpecBytes is the raw OpenAPI spec content. If set, SpecPath is used only
	// for display purposes.
	SpecBytes []byte

	// Port is the TCP port to listen on.
	Port int

	// Mode is the operating mode (deterministic or stateful).
	Mode model.OperatingMode

	// Strict enables strict response validation.
	Strict bool

	// MaxDepth is the maximum recursion depth for data generation.
	MaxDepth int
}

// StatusResult holds the current state of the Mimikos instance.
type StatusResult struct {
	// Running is true when a mock server is actively listening.
	Running bool

	// Port is the TCP port the server is listening on. Zero when stopped.
	Port int

	// Mode is the operating mode. Empty when stopped.
	Mode model.OperatingMode

	// SpecTitle is the OpenAPI spec title. Empty when stopped.
	SpecTitle string

	// SpecVersion is the OpenAPI spec version string. Empty when stopped.
	SpecVersion string

	// Operations is the number of classified endpoints. Zero when stopped.
	Operations int

	// UptimeSeconds is how long the server has been running. Zero when stopped.
	UptimeSeconds float64
}

// Instance holds the mutable state of a single running Mimikos mock server.
// At most one mock server is active at a time (single-instance model).
// All methods are safe for concurrent use.
type Instance struct {
	mu            sync.Mutex
	running       bool
	srv           *http.Server
	handler       http.Handler
	behaviorMap   *model.BehaviorMap
	startupResult *server.StartupResult
	store         state.Store
	startedAt     time.Time
	port          int
	mode          model.OperatingMode
	specTitle     string
	specVersion   string
}

// Start reads the OpenAPI spec, builds the HTTP handler via server.Build, binds
// a TCP listener, and starts serving in a background goroutine. Returns an error
// if the instance is already running, the spec is invalid, or the port is
// unavailable.
func (inst *Instance) Start(ctx context.Context, cfg StartConfig) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.running {
		return fmt.Errorf(
			"%w on port %d — call stop_server first",
			ErrAlreadyRunning, inst.port,
		)
	}

	// Resolve operating mode default.
	mode := cfg.Mode
	if mode == "" {
		mode = model.ModeDeterministic
	}

	// Build the HTTP handler from the spec.
	handler, result, err := server.Build(ctx, cfg.SpecBytes, server.Config{
		Strict:   cfg.Strict,
		MaxDepth: cfg.MaxDepth,
		Mode:     mode,
	})
	if err != nil {
		return fmt.Errorf("cannot start server: %w", err)
	}

	// Bind the TCP listener to confirm the port is available.
	addr := fmt.Sprintf(":%d", cfg.Port)

	var lc net.ListenConfig

	listener, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is not available: %w", cfg.Port, err)
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Start serving in the background.
	go func() {
		_ = srv.Serve(listener)
	}()

	// Record instance state.
	inst.running = true
	inst.srv = srv
	inst.handler = handler
	inst.behaviorMap = result.BehaviorMap
	inst.startupResult = result
	inst.port = cfg.Port
	inst.mode = mode
	inst.specTitle = result.SpecTitle
	inst.specVersion = result.SpecVersion
	inst.startedAt = timeNow()

	return nil
}

// Stop gracefully shuts down the running mock server. Returns an error if no
// server is running.
func (inst *Instance) Stop(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.running {
		return fmt.Errorf("%w — call start_server first", ErrNotRunning)
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	err := inst.srv.Shutdown(shutdownCtx)

	// Clear state regardless of shutdown outcome.
	inst.running = false
	inst.srv = nil
	inst.handler = nil
	inst.behaviorMap = nil
	inst.startupResult = nil
	inst.store = nil
	inst.port = 0
	inst.mode = ""
	inst.specTitle = ""
	inst.specVersion = ""
	inst.startedAt = time.Time{}

	if err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	return nil
}

// Status returns the current state of the instance. Always succeeds — returns
// Running: false when no server is active.
func (inst *Instance) Status() StatusResult {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if !inst.running {
		return StatusResult{Running: false}
	}

	return StatusResult{
		Running:       true,
		Port:          inst.port,
		Mode:          inst.mode,
		SpecTitle:     inst.specTitle,
		SpecVersion:   inst.specVersion,
		Operations:    inst.behaviorMap.Len(),
		UptimeSeconds: timeNow().Sub(inst.startedAt).Seconds(),
	}
}

// IsRunning reports whether a mock server is currently active.
func (inst *Instance) IsRunning() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	return inst.running
}

// parseMode converts a mode string to model.OperatingMode with a
// user-friendly error message for invalid values.
func parseMode(s string) (model.OperatingMode, error) {
	m, err := model.ParseOperatingMode(s)
	if err != nil {
		return "", fmt.Errorf(
			"%w %q — must be \"deterministic\" or \"stateful\"",
			ErrInvalidMode, s,
		)
	}

	return m, nil
}
