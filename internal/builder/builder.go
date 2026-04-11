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

	// Step 2: Determine success code.
	successCode := selectSuccessCode(op.Responses, result.Type)

	// Step 3: Build response schemas map and compile schemas.
	var requestSchema *model.CompiledSchema

	responseSchemas := buildResponseSchemas(op, sc, logger)

	if sc != nil {
		requestSchema = compileRequestSchema(op, sc, logger)
	}

	return model.BehaviorEntry{
		OperationID:      op.OperationID,
		Method:           op.Method,
		PathPattern:      op.Path,
		Type:             result.Type,
		SuccessCode:      successCode,
		RequestSchema:    requestSchema,
		ResponseSchemas:  responseSchemas,
		ResponseExamples: buildResponseExamples(op),
		BodyRequired:     op.RequestBody != nil && op.RequestBody.Required,
		Source:           model.SourceHeuristic,
		Confidence:       result.Confidence,
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

// buildResponseSchemas builds the response schemas map from an operation's
// responses. Every response defined in the spec gets a key in the map:
// compiled schema if it has one, nil if it doesn't (preserving "this status
// code exists" information). The default response uses key 0.
// When sc is nil, schema compilation is skipped but status code presence
// is still recorded.
func buildResponseSchemas(
	op parser.Operation,
	sc *compiler.SchemaCompiler,
	logger *slog.Logger,
) map[int]*model.CompiledSchema {
	var schemas map[int]*model.CompiledSchema

	for code, resp := range op.Responses {
		if resp == nil {
			continue
		}

		if schemas == nil {
			schemas = make(map[int]*model.CompiledSchema)
		}

		if resp.Schema == nil || sc == nil {
			schemas[code] = nil

			continue
		}

		compiled := compileSchema(sc, resp.Schema, op, logger)
		schemas[code] = compiled
	}

	// Default response at key 0 — same logic as the status-code loop above:
	// key presence means "defined in spec," nil value means "no schema."
	if op.DefaultResponse != nil {
		if schemas == nil {
			schemas = make(map[int]*model.CompiledSchema)
		}

		if op.DefaultResponse.Schema == nil || sc == nil {
			schemas[0] = nil
		} else {
			schemas[0] = compileSchema(sc, op.DefaultResponse.Schema, op, logger)
		}
	}

	return schemas
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
