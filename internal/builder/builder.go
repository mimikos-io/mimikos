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
	"sort"

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
)

// successCodeDefault is used when the spec defines no responses for an operation.
const successCodeDefault = http.StatusOK

// BuildBehaviorMap classifies every operation in the parsed spec, compiles
// referenced schemas, infers scenarios, and assembles a complete BehaviorMap.
//
// The compiler is nil-safe: if sc is nil, schema compilation is skipped and
// all schema fields in BehaviorEntry are nil. The logger is nil-safe: if
// logger is nil, warnings are discarded.
func BuildBehaviorMap(
	spec *parser.ParsedSpec,
	cls *classifier.Classifier,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) (*model.BehaviorMap, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}

	if cls == nil {
		return nil, ErrNilClassifier
	}

	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	bm := model.NewBehaviorMap()

	for _, op := range spec.Operations {
		entry := buildEntry(op, cls, sc, logger)

		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %s %s: %w", ErrInvalidEntry, op.Method, op.Path, err)
		}

		bm.Put(entry)
	}

	return bm, nil
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

	// Step 2: Infer scenarios.
	scenarios := classifier.InferScenarios(result.Type)

	// Step 3: Determine success code.
	successCode := selectSuccessCode(op.Responses, result.Type)

	// Step 4: Collect error codes.
	errorCodes := collectErrorCodes(op.Responses)

	// Step 5: Compile schemas.
	var (
		requestSchema   *model.CompiledSchema
		responseSchemas map[int]*model.CompiledSchema
	)

	if sc != nil {
		requestSchema = compileRequestSchema(op, sc, logger)
		responseSchemas = compileResponseSchemas(op, sc, logger)
	}

	return model.BehaviorEntry{
		OperationID:     op.OperationID,
		Method:          op.Method,
		PathPattern:     op.Path,
		Type:            result.Type,
		Scenarios:       scenarios,
		SuccessCode:     successCode,
		ErrorCodes:      errorCodes,
		RequestSchema:   requestSchema,
		ResponseSchemas: responseSchemas,
		Source:          model.SourceHeuristic,
		Confidence:      result.Confidence,
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

// collectErrorCodes returns all 4xx/5xx status codes from the response map,
// sorted in ascending order for deterministic output.
func collectErrorCodes(responses map[int]*parser.Response) []int {
	var codes []int

	for code := range responses {
		if code >= http.StatusBadRequest {
			codes = append(codes, code)
		}
	}

	sort.Ints(codes)

	return codes
}

// compileRequestSchema compiles the request body schema, if present.
func compileRequestSchema(
	op parser.Operation,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) *model.CompiledSchema {
	if op.RequestBody == nil || op.RequestBody.Schema == nil {
		return nil
	}

	ref := op.RequestBody.Schema

	compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
	if err != nil {
		logger.Warn("failed to compile request schema",
			"operation", op.OperationID,
			"method", op.Method,
			"path", op.Path,
			"pointer", ref.Pointer,
			"error", err,
		)

		return nil
	}

	return compiled
}

// compileResponseSchemas compiles all response schemas (including default)
// into a map keyed by status code. The default response uses key 0.
func compileResponseSchemas(
	op parser.Operation,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) map[int]*model.CompiledSchema {
	var schemas map[int]*model.CompiledSchema

	for code, resp := range op.Responses {
		if resp == nil || resp.Schema == nil {
			continue
		}

		compiled := compileSchema(sc, resp.Schema, op, logger)
		if compiled != nil {
			if schemas == nil {
				schemas = make(map[int]*model.CompiledSchema)
			}

			schemas[code] = compiled
		}
	}

	// Default response at key 0.
	if op.DefaultResponse != nil && op.DefaultResponse.Schema != nil {
		compiled := compileSchema(sc, op.DefaultResponse.Schema, op, logger)
		if compiled != nil {
			if schemas == nil {
				schemas = make(map[int]*model.CompiledSchema)
			}

			schemas[0] = compiled
		}
	}

	return schemas
}

// compileSchema compiles a single SchemaRef, logging a warning on failure.
func compileSchema(
	sc *compiler.SchemaCompiler,
	ref *parser.SchemaRef,
	op parser.Operation,
	logger *slog.Logger,
) *model.CompiledSchema {
	compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
	if err != nil {
		logger.Warn("failed to compile response schema",
			"operation", op.OperationID,
			"method", op.Method,
			"path", op.Path,
			"pointer", ref.Pointer,
			"error", err,
		)

		return nil
	}

	return compiled
}
