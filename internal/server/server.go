// Package server wires the Mimikos startup pipeline from raw OpenAPI spec
// bytes into a ready-to-serve http.Handler. It connects the parser, classifier,
// compiler, builder, validator, generator, error responder, and router.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/pb33f/libopenapi"

	"github.com/mimikos-io/mimikos/internal/builder"
	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/compiler"
	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/mimikos-io/mimikos/internal/router"
	"github.com/mimikos-io/mimikos/internal/state"
	"github.com/mimikos-io/mimikos/internal/validator"
)

type (
	// Config holds all configuration for building and starting the server.
	Config struct {
		// Strict enables strict response validation mode. When true, responses
		// that fail schema validation return 500 instead of a warning log.
		Strict bool

		// MaxDepth is the maximum recursion depth for data generation.
		// Zero uses the default (3).
		MaxDepth int

		// Logger is the structured logger for startup diagnostics.
		// Nil uses a no-op logger.
		Logger *slog.Logger

		// Mode selects the operating mode. Default (zero value) is deterministic.
		Mode model.OperatingMode

		// MaxResources is the state store capacity for stateful mode.
		// Zero uses the default (10,000).
		MaxResources int
	}

	// discardHandler is a slog.Handler that discards all log output.
	discardHandler struct{}
)

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// StartupResult carries diagnostic info produced during server assembly
// for CLI startup logging.
type StartupResult struct {
	SpecTitle       string
	SpecVersion     string
	Operations      int
	FailedEntries   int
	FailedPaths     []string // "METHOD /path" for each failed endpoint
	DegradedSchemas int
	DegradedPaths   []string // "METHOD /path" for each degraded endpoint
	Entries         []EntryInfo
	Mode            model.OperatingMode
}

// EntryInfo holds per-operation info for startup logging.
type EntryInfo struct {
	Method       string
	PathPattern  string
	BehaviorType string
	Confidence   string
}

// Build constructs the HTTP handler from raw spec bytes. It runs the full
// startup pipeline: parse → classify → compile → build behavior map → wire
// router. Returns the handler, startup diagnostics, or an error.
func Build(ctx context.Context, specBytes []byte, cfg Config) (http.Handler, *StartupResult, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	p := parser.NewLibopenAPIParser(logger)

	spec, err := p.Parse(ctx, doc)
	if err != nil {
		return nil, nil, fmt.Errorf("parse spec: %w", err)
	}

	sc, err := compiler.New(specBytes, spec.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("init schema compiler: %w", err)
	}

	classify := classifier.New()

	bm, failed, err := builder.BuildBehaviorMap(spec, classify, sc, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("build behavior map: %w", err)
	}

	for _, f := range failed {
		logger.Warn("endpoint not registered — panicked during startup",
			"method", f.Method,
			"path", f.PathPattern,
			"error", f.Error,
		)
	}

	// Annotate stateful metadata (wrapper keys, list array keys, ID field hints)
	// before validators and router consume the behavior map.
	if cfg.Mode == model.ModeStateful {
		annotateStatefulMetadata(bm, logger)
	}

	v, err := validator.NewLibopenAPIValidator(doc)
	if err != nil {
		return nil, nil, fmt.Errorf("init request validator: %w", err)
	}

	// Create remaining runtime dependencies.
	responder := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), cfg.MaxDepth, logger)

	// Resolve operating mode — default to deterministic.
	mode := cfg.Mode
	if mode == "" {
		mode = model.ModeDeterministic
	}

	// Create state store for stateful mode.
	var store state.Store

	if mode == model.ModeStateful {
		capacity := cfg.MaxResources
		if capacity == 0 {
			capacity = 10_000
		}

		store = state.NewInMemory(capacity)
	}

	// Wire router.
	handler := router.NewHandler(bm, failed, v, responder, gen, cfg.Strict, logger, mode, store)

	return handler, buildStartupResult(spec, bm, failed, mode), nil
}

// buildStartupResult collects diagnostic info from the startup pipeline.
func buildStartupResult(
	spec *parser.ParsedSpec,
	bm *model.BehaviorMap,
	failed []builder.FailedEntry,
	mode model.OperatingMode,
) *StartupResult {
	entries := make([]EntryInfo, 0, bm.Len())

	var degradedPaths []string

	for _, e := range bm.Entries() {
		entries = append(entries, EntryInfo{
			Method:       e.Method,
			PathPattern:  e.PathPattern,
			BehaviorType: e.Type.String(),
			Confidence:   classifier.ConfidenceLabel(e.Confidence),
		})

		if e.DegradedResponseSchema != "" || e.DegradedRequestSchema != "" {
			degradedPaths = append(degradedPaths, e.Method+" "+e.PathPattern)
		}
	}

	failedPaths := make([]string, len(failed))
	for i, f := range failed {
		failedPaths[i] = f.Method + " " + f.PathPattern
	}

	return &StartupResult{
		SpecTitle:       spec.Title,
		SpecVersion:     spec.Version,
		Operations:      bm.Len(),
		FailedEntries:   len(failed),
		FailedPaths:     failedPaths,
		DegradedSchemas: len(degradedPaths),
		DegradedPaths:   degradedPaths,
		Entries:         entries,
		Mode:            mode,
	}
}
