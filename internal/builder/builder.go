// Package builder wires the startup pipeline that transforms a parsed OpenAPI
// spec into a complete BehaviorMap. It connects the classifier, schema
// compiler, and scenario inference into a single BuildBehaviorMap call.
//
// The builder is glue code — it depends on parser, classifier, compiler, and
// model, but nothing depends on it except the CLI entry point.
package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/compiler"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// discardHandler is a slog.Handler that discards all log output.
type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler      { return d }
func (d discardHandler) WithGroup(string) slog.Handler           { return d }

// Sentinel errors for builder failures.
var (
	// ErrNilSpec is returned when a nil ParsedSpec is passed to BuildBehaviorMap.
	ErrNilSpec = errors.New("builder: spec is nil")

	// ErrNilClassifier is returned when a nil Classifier is passed to BuildBehaviorMap.
	ErrNilClassifier = errors.New("builder: classifier is nil")

	// ErrInvalidEntry is returned when the builder produces a BehaviorEntry that
	// fails validation. This indicates a bug in the builder's assembly logic.
	ErrInvalidEntry = errors.New("builder: invalid behavior entry")

	// ErrEntryPanicked is returned when buildEntry panics on a spec edge case.
	ErrEntryPanicked = errors.New("builder: entry panicked")
)

// FailedEntry records an operation that could not be processed during startup.
// This happens when buildEntry panics on a spec edge case (nil pointer, unexpected
// schema shape). The server starts without these endpoints and serves an actionable
// error if a client requests them.
type FailedEntry struct {
	Method      string
	PathPattern string
	Error       string // recovered panic message
}

// successCodeDefault is used when the spec defines no responses for an operation.
const successCodeDefault = http.StatusOK

// BuildBehaviorMap classifies every operation in the parsed spec, compiles
// referenced schemas, infers scenarios, and assembles a complete BehaviorMap.
//
// Operations that panic during processing are recovered, logged, and returned
// as FailedEntry values. The BehaviorMap contains only successfully processed
// entries, allowing the server to start with partial coverage.
//
// The compiler is nil-safe: if sc is nil, schema compilation is skipped and
// all schema fields in BehaviorEntry are nil. The logger is nil-safe: if
// logger is nil, warnings are discarded.
func BuildBehaviorMap(
	spec *parser.ParsedSpec,
	cls *classifier.Classifier,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) (*model.BehaviorMap, []FailedEntry, error) {
	if spec == nil {
		return nil, nil, ErrNilSpec
	}

	if cls == nil {
		return nil, nil, ErrNilClassifier
	}

	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	bm := model.NewBehaviorMap()

	var failed []FailedEntry

	for _, op := range spec.Operations {
		entry, err := safeBuildEntry(op, cls, sc, logger)
		if err != nil {
			logger.Warn("endpoint panicked during startup — skipping",
				"method", op.Method,
				"path", op.Path,
				"error", err,
			)

			failed = append(failed, FailedEntry{
				Method:      op.Method,
				PathPattern: op.Path,
				Error:       err.Error(),
			})

			continue
		}

		if valErr := entry.Validate(); valErr != nil {
			return nil, nil, fmt.Errorf("%w: %s %s: %w", ErrInvalidEntry, op.Method, op.Path, valErr)
		}

		bm.Put(entry)
	}

	return bm, failed, nil
}

// safeBuildEntry wraps buildEntry with panic recovery. Returns the entry on
// success, or an error describing the recovered panic.
//
//nolint:nonamedreturns // named returns required for defer/recover to set the error
func safeBuildEntry(
	op parser.Operation,
	cls *classifier.Classifier,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) (entry model.BehaviorEntry, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrEntryPanicked, r)
		}
	}()

	return buildEntry(op, cls, sc, logger), nil
}

// buildEntry assembles a single BehaviorEntry from an operation by running
// classification, scenario inference, status code selection, and schema
// compilation.
func buildEntry(
	op parser.Operation,
	cls *classifier.Classifier,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) model.BehaviorEntry {
	// Step 1: Classify.
	result := cls.Classify(op)

	// Step 2: Determine success code.
	successCode := selectSuccessCode(op.Responses, result.Type)

	// Step 3: Build response schemas map and compile schemas.
	var requestSchema *model.CompiledSchema

	var degradedRequest string

	responseSchemas, responseErrors := buildResponseSchemas(op, sc, logger)

	if sc != nil {
		requestSchema, degradedRequest = compileRequestSchema(op, sc, logger)
	}

	// Step 4: Record degradation for the success code's response schema.
	// Only the success code matters — error schemas have writeErrorFallback.
	degradedResponse := responseErrors[successCode]

	return model.BehaviorEntry{
		OperationID:            op.OperationID,
		Method:                 op.Method,
		PathPattern:            op.Path,
		Type:                   result.Type,
		SuccessCode:            successCode,
		RequestSchema:          requestSchema,
		ResponseSchemas:        responseSchemas,
		ResponseExamples:       buildResponseExamples(op),
		BodyRequired:           op.RequestBody != nil && op.RequestBody.Required,
		Source:                 model.SourceHeuristic,
		Confidence:             result.Confidence,
		DegradedResponseSchema: degradedResponse,
		DegradedRequestSchema:  degradedRequest,
	}
}

// selectSuccessCode picks the HTTP status code for the success scenario.
// Rules:
//   - create prefers 201
//   - delete prefers 204
//   - all others prefer 200
//   - fallback: lowest defined 2xx
//   - default: 200 if no responses defined
func selectSuccessCode(responses map[int]*parser.Response, bt model.BehaviorType) int {
	if len(responses) == 0 {
		return successCodeDefault
	}

	preferred := preferredSuccessCode(bt)

	// Check if the preferred code is defined.
	if _, ok := responses[preferred]; ok {
		return preferred
	}

	// Fallback: lowest 2xx code defined in the spec.
	lowest := 0

	for code := range responses {
		if code >= http.StatusOK && code < http.StatusMultipleChoices {
			if lowest == 0 || code < lowest {
				lowest = code
			}
		}
	}

	if lowest > 0 {
		return lowest
	}

	return successCodeDefault
}

// preferredSuccessCode returns the conventional success status code for a
// behavior type.
func preferredSuccessCode(bt model.BehaviorType) int {
	switch bt {
	case model.BehaviorCreate:
		return http.StatusCreated
	case model.BehaviorDelete:
		return http.StatusNoContent
	case model.BehaviorFetch, model.BehaviorList, model.BehaviorUpdate, model.BehaviorGeneric:
		return http.StatusOK
	}

	return http.StatusOK
}

// compileRequestSchema compiles the request body schema, if present.
// Returns the compiled schema and the error message (empty on success or
// when no request body schema is defined).
func compileRequestSchema(
	op parser.Operation,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) (*model.CompiledSchema, string) {
	if op.RequestBody == nil || op.RequestBody.Schema == nil {
		return nil, ""
	}

	ref := op.RequestBody.Schema

	compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
	if err != nil {
		logger.Warn("request schema failed to compile — request body validation will be skipped for this endpoint",
			"operation", op.OperationID,
			"method", op.Method,
			"path", op.Path,
			"pointer", ref.Pointer,
			"error", err,
		)

		return nil, err.Error()
	}

	return compiled, ""
}

// responseSchemaResult collects compiled response schemas and any compilation
// errors. Used by buildResponseSchemas to accumulate results without deep nesting.
type responseSchemaResult struct {
	schemas map[int]*model.CompiledSchema
	errors  map[int]string
}

// set records a compiled schema (possibly nil) for a status code. If errMsg is
// non-empty, it is recorded as a compilation error for that code.
func (r *responseSchemaResult) set(code int, schema *model.CompiledSchema, errMsg string) {
	if r.schemas == nil {
		r.schemas = make(map[int]*model.CompiledSchema)
	}

	r.schemas[code] = schema

	if errMsg != "" {
		if r.errors == nil {
			r.errors = make(map[int]string)
		}

		r.errors[code] = errMsg
	}
}

// buildResponseSchemas builds the response schemas map from an operation's
// responses. Every response defined in the spec gets a key in the map:
// compiled schema if it has one, nil if it doesn't (preserving "this status
// code exists" information). The default response uses key 0.
// When sc is nil, schema compilation is skipped but status code presence
// is still recorded.
//
// The second return value maps status codes to compilation error messages.
// It is nil when all schemas compile successfully (zero allocation).
func buildResponseSchemas(
	op parser.Operation,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) (map[int]*model.CompiledSchema, map[int]string) {
	var result responseSchemaResult

	for code, resp := range op.Responses {
		if resp == nil {
			continue
		}

		if resp.Schema == nil || sc == nil {
			result.set(code, nil, "")

			continue
		}

		compiled, errMsg := compileSchema(sc, resp.Schema, op, logger)
		result.set(code, compiled, errMsg)
	}

	// Default response at key 0 — key presence means "defined in spec."
	if op.DefaultResponse != nil {
		if op.DefaultResponse.Schema == nil || sc == nil {
			result.set(0, nil, "")
		} else {
			compiled, errMsg := compileSchema(sc, op.DefaultResponse.Schema, op, logger)
			result.set(0, compiled, errMsg)
		}
	}

	return result.schemas, result.errors
}

// buildResponseExamples collects media-type examples from an operation's
// responses into a map keyed by status code (0 for default). Returns nil
// if no responses have examples (zero allocation).
func buildResponseExamples(op parser.Operation) map[int]any {
	var examples map[int]any

	for code, resp := range op.Responses {
		if resp == nil || resp.Example == nil {
			continue
		}

		if examples == nil {
			examples = make(map[int]any)
		}

		examples[code] = resp.Example
	}

	// Default response example at key 0.
	if op.DefaultResponse != nil && op.DefaultResponse.Example != nil {
		if examples == nil {
			examples = make(map[int]any)
		}

		examples[0] = op.DefaultResponse.Example
	}

	return examples
}

// compileSchema compiles a single SchemaRef, logging a warning on failure.
// Returns the compiled schema and the error message (empty on success).
func compileSchema(
	sc *compiler.SchemaCompiler,
	ref *parser.SchemaRef,
	op parser.Operation,
	logger *slog.Logger,
) (*model.CompiledSchema, string) {
	compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
	if err != nil {
		logger.Warn("response schema failed to compile — endpoint will return an error instead of generated data",
			"operation", op.OperationID,
			"method", op.Method,
			"path", op.Path,
			"pointer", ref.Pointer,
			"error", err,
		)

		return nil, err.Error()
	}

	return compiled, ""
}
