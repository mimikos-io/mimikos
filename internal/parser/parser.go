// Package parser implements the OpenAPI spec parser and schema resolver.
// It wraps libopenapi to parse OpenAPI 3.0/3.1 specs into a normalized
// internal model with resolved references and circular ref annotations.
package parser

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
)

// LibopenAPIParser parses OpenAPI specs using libopenapi.
type LibopenAPIParser struct {
	logger *slog.Logger
}

// NewLibopenAPIParser creates a parser with the given logger.
// If logger is nil, a no-op logger is used (discards all output).
func NewLibopenAPIParser(logger *slog.Logger) *LibopenAPIParser {
	if logger == nil {
		logger = slog.New(discardHandler{})
	}

	return &LibopenAPIParser{logger: logger}
}

// Parse parses a pre-built libopenapi Document into a normalized ParsedSpec.
// The caller is responsible for creating the Document from raw spec bytes via
// libopenapi.NewDocument. This avoids double-parsing when the Document is
// shared with other consumers (e.g., the request validator).
func (p *LibopenAPIParser) Parse(ctx context.Context, doc libopenapi.Document) (*ParsedSpec, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	version := doc.GetVersion()
	if err := validateVersion(version); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	model, buildErr := doc.BuildV3Model()
	if model == nil {
		if buildErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrBuildModel, buildErr)
		}

		return nil, ErrBuildModel
	}

	if buildErr != nil {
		p.logger.Warn("OpenAPI spec parsed with warnings — some schema definitions may not resolve correctly",
			"error", buildErr)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Collect circular reference info.
	circularSet, circularRefs := p.collectCircularRefs(model)

	// Extract operations.
	operations := p.extractOperations(model.Model, circularSet)

	return &ParsedSpec{
		Version:      version,
		Title:        model.Model.Info.Title,
		Operations:   operations,
		CircularRefs: circularRefs,
	}, nil
}

// validateVersion checks that the spec version is supported (OpenAPI 3.x).
func validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("%w: version is empty", ErrInvalidSpec)
	}

	if strings.HasPrefix(version, "2.") || strings.HasPrefix(version, "1.") {
		return fmt.Errorf("%w: %s (only OpenAPI 3.x is supported)", ErrUnsupportedVersion, version)
	}

	if !strings.HasPrefix(version, "3.") {
		return fmt.Errorf("%w: %s", ErrUnsupportedVersion, version)
	}

	return nil
}

// collectCircularRefs extracts circular reference information from the model index
// and returns a set of schema names involved in cycles (for per-schema annotation)
// plus a list of CircularRef descriptors (for diagnostics).
func (p *LibopenAPIParser) collectCircularRefs(
	model *libopenapi.DocumentModel[v3high.Document],
) (map[string]bool, []CircularRef) {
	circularResults := model.Index.GetCircularReferences()
	if len(circularResults) == 0 {
		return nil, nil
	}

	circularSet := make(map[string]bool, len(circularResults))
	refs := make([]CircularRef, 0, len(circularResults))

	for _, cr := range circularResults {
		// Extract schema name from the start reference.
		name := schemaNameFromRef(cr.Start.Definition)
		circularSet[name] = true

		// Also mark all schemas in the journey.
		for _, ref := range cr.Journey {
			jName := schemaNameFromRef(ref.Definition)
			if jName != "" {
				circularSet[jName] = true
			}
		}

		refs = append(refs, CircularRef{
			SchemaName:     name,
			JourneyPath:    cr.GenerateJourneyPath(),
			IsInfiniteLoop: cr.IsInfiniteLoop,
			IsPolymorphic:  cr.IsPolymorphicResult,
			IsArray:        cr.IsArrayResult,
		})

		p.logger.Info("circular reference detected",
			"schema", name,
			"infinite", cr.IsInfiniteLoop,
			"journey", cr.GenerateJourneyPath(),
		)
	}

	return circularSet, refs
}

// extractOperations iterates all paths and operations in spec source order,
// building the Operation slice.
func (p *LibopenAPIParser) extractOperations(
	doc v3high.Document, circularSet map[string]bool,
) []Operation {
	if doc.Paths == nil || doc.Paths.PathItems == nil {
		return nil
	}

	var operations []Operation

	for pathPair := doc.Paths.PathItems.Oldest(); pathPair != nil; pathPair = pathPair.Next() {
		path := pathPair.Key
		pathItem := pathPair.Value

		ops := pathItem.GetOperations()
		if ops == nil {
			continue
		}

		for opPair := ops.Oldest(); opPair != nil; opPair = opPair.Next() {
			method := strings.ToUpper(opPair.Key)
			v3Op := opPair.Value

			op := Operation{
				Method:      method,
				Path:        path,
				OperationID: v3Op.OperationId,
				Summary:     v3Op.Summary,
				Description: v3Op.Description,
				Tags:        v3Op.Tags,
			}

			// Build JSON pointer prefix for this operation's schemas.
			opPointer := "#/paths/" + encodeJSONPointerToken(path) + "/" + opPair.Key

			// Extract request body.
			op.RequestBody = p.extractRequestBody(v3Op, circularSet, opPointer)

			// Extract responses.
			op.Responses, op.DefaultResponse = p.extractResponses(v3Op, circularSet, opPointer)

			operations = append(operations, op)
		}
	}

	return operations
}

// extractRequestBody extracts the JSON request body schema from an operation.
func (p *LibopenAPIParser) extractRequestBody(
	op *v3high.Operation, circularSet map[string]bool, pathPointer string,
) *RequestBody {
	if op.RequestBody == nil || op.RequestBody.Content == nil {
		return nil
	}

	mt, contentType := findJSONMediaType(op.RequestBody.Content)
	if mt == nil || mt.Schema == nil {
		return nil
	}

	required := op.RequestBody.Required != nil && *op.RequestBody.Required
	inlinePointer := pathPointer + "/requestBody/content/" + encodeJSONPointerToken(contentType) + "/schema"

	return &RequestBody{
		Required: required,
		Schema:   p.resolveSchemaRef(mt.Schema, circularSet, inlinePointer),
	}
}

// extractResponses extracts JSON response schemas keyed by status code,
// plus the default response if present.
func (p *LibopenAPIParser) extractResponses(
	op *v3high.Operation, circularSet map[string]bool, pathPointer string,
) (map[int]*Response, *Response) {
	if op.Responses == nil {
		return nil, nil
	}

	var responses map[int]*Response

	if op.Responses.Codes != nil {
		responses = make(map[int]*Response, op.Responses.Codes.Len())

		for codePair := op.Responses.Codes.Oldest(); codePair != nil; codePair = codePair.Next() {
			code, err := strconv.Atoi(codePair.Key)
			if err != nil {
				p.logger.Warn("non-numeric response code ignored — only integer status codes (e.g., 200, 404) are supported",
					"code", codePair.Key,
					"operation", op.OperationId,
				)

				continue
			}

			v3Resp := codePair.Value
			resp := &Response{
				StatusCode:  code,
				Description: v3Resp.Description,
			}

			schemaPointer := pathPointer + "/responses/" + codePair.Key
			p.extractResponseContent(resp, v3Resp.Content, circularSet, schemaPointer, op.OperationId, codePair.Key)

			responses[code] = resp
		}
	}

	// Default response.
	var defaultResp *Response

	if op.Responses.Default != nil {
		v3Default := op.Responses.Default
		defaultResp = &Response{
			StatusCode:  0,
			Description: v3Default.Description,
		}

		schemaPointer := pathPointer + "/responses/default"
		p.extractResponseContent(defaultResp, v3Default.Content, circularSet, schemaPointer, op.OperationId, "default")
	}

	return responses, defaultResp
}

// extractResponseContent populates a Response's Schema and Example fields from
// the content map. Extracted to reduce nesting in extractResponses.
func (p *LibopenAPIParser) extractResponseContent(
	resp *Response,
	content *orderedmap.Map[string, *v3high.MediaType],
	circularSet map[string]bool,
	basePointer string,
	operationID string,
	responseCode string,
) {
	if content == nil {
		return
	}

	mt, contentType := findJSONMediaType(content)
	if mt == nil {
		return
	}

	if mt.Schema != nil {
		inlinePointer := basePointer + "/content/" + encodeJSONPointerToken(contentType) + "/schema"
		resp.Schema = p.resolveSchemaRef(mt.Schema, circularSet, inlinePointer)
	}

	resp.Example = p.extractMediaTypeExample(mt, operationID, responseCode)
}

// resolveSchemaRef resolves a SchemaProxy into a SchemaRef with name,
// circular reference annotation, and JSON pointer for the Schema Compiler.
//
// The inlinePointer is the JSON pointer to the schema's location in the spec
// (used for inline schemas). For $ref schemas, the pointer is derived from
// the reference target instead.
func (p *LibopenAPIParser) resolveSchemaRef(
	proxy *base.SchemaProxy, circularSet map[string]bool, inlinePointer string,
) *SchemaRef {
	if proxy == nil {
		return nil
	}

	schema := proxy.Schema()
	if schema == nil {
		p.logger.Warn("$ref target resolved to empty schema — referenced definition may be missing or malformed in spec",
			"ref", proxy.GetReference())

		return nil
	}

	name := schemaNameFromProxy(proxy)

	// For $ref schemas, use the reference target as the pointer.
	// For inline schemas, use the caller-provided pointer.
	pointer := inlinePointer
	if ref := proxy.GetReference(); ref != "" {
		pointer = ref
	}

	return &SchemaRef{
		Name:       name,
		IsCircular: circularSet[name],
		Raw:        schema,
		Pointer:    pointer,
	}
}

// findJSONMediaType finds the MediaType object for application/json (or
// compatible JSON content type) in a content map. Returns the MediaType and
// the matched content type string. Returns (nil, "") if no JSON content type
// found.
func findJSONMediaType(
	content *orderedmap.Map[string, *v3high.MediaType],
) (*v3high.MediaType, string) {
	if content == nil {
		return nil, ""
	}

	// Try exact match first.
	if mt := content.GetOrZero("application/json"); mt != nil {
		return mt, "application/json"
	}

	// Fall back to any application/*+json variant.
	for pair := content.Oldest(); pair != nil; pair = pair.Next() {
		ct := pair.Key
		if strings.HasPrefix(ct, "application/") && strings.HasSuffix(ct, "+json") {
			return pair.Value, ct
		}
	}

	return nil, ""
}

// extractMediaTypeExample extracts a complete response example from a Media
// Type Object. Priority: singular `example` first, then first entry from
// plural `examples`. Returns nil if no example is available.
// externalValue-only examples are ignored.
func (p *LibopenAPIParser) extractMediaTypeExample(
	mt *v3high.MediaType, operationID string, responseCode string,
) any {
	// 1. Singular example — highest priority.
	if mt.Example != nil {
		var val any
		if err := mt.Example.Decode(&val); err != nil {
			p.logger.Warn("failed to decode media-type example — example will be ignored; "+
				"check the YAML syntax in the example value",
				"operation", operationID,
				"response", responseCode,
				"error", err,
			)

			return nil
		}

		return normalizeYAMLTypes(val)
	}

	// 2. Plural examples — use first entry's Value (spec order preserved by orderedmap).
	if mt.Examples != nil {
		for pair := mt.Examples.Oldest(); pair != nil; pair = pair.Next() {
			example := pair.Value
			if example == nil || example.Value == nil {
				// Skip externalValue-only or empty examples.
				continue
			}

			var val any
			if err := example.Value.Decode(&val); err != nil {
				p.logger.Warn("failed to decode named media-type example — example will be ignored; "+
					"check the YAML syntax in the example value",
					"operation", operationID,
					"response", responseCode,
					"name", pair.Key,
					"error", err,
				)

				return nil
			}

			return normalizeYAMLTypes(val)
		}
	}

	return nil
}

// normalizeYAMLTypes recursively converts YAML-native Go types to types
// consistent with the generator output. Specifically, yaml.Node.Decode
// produces Go `int` for YAML integers, but the generator produces `int64`.
// On the wire (JSON) they're identical, but Go-level comparisons differ.
// This function normalizes int → int64 throughout the value tree so
// downstream code (router, tests) doesn't need to worry about it.
//
// Note: map and slice values are mutated in place for efficiency. The caller
// must not reuse the input if it needs the original YAML-native types.
func normalizeYAMLTypes(v any) any {
	switch val := v.(type) {
	case int:
		return int64(val)
	case map[string]any:
		for k, elem := range val {
			val[k] = normalizeYAMLTypes(elem)
		}

		return val
	case []any:
		for i, elem := range val {
			val[i] = normalizeYAMLTypes(elem)
		}

		return val
	default:
		return v
	}
}

// schemaNameFromProxy extracts a human-readable name from a SchemaProxy.
// For $ref proxies, it extracts the component name from the reference path.
// For inline schemas, it returns an empty string (caller should generate a name).
func schemaNameFromProxy(proxy *base.SchemaProxy) string {
	ref := proxy.GetReference()
	if ref != "" {
		return schemaNameFromRef(ref)
	}

	return ""
}

// schemaNameFromRef extracts the schema name from a $ref string.
// e.g., "#/components/schemas/Pet" → "Pet".
func schemaNameFromRef(ref string) string {
	if ref == "" {
		return ""
	}

	parts := strings.Split(ref, "/")

	return parts[len(parts)-1]
}

// encodeJSONPointerToken encodes a string for use as a JSON Pointer token
// per RFC 6901. The characters '~' and '/' must be escaped as '~0' and '~1'.
func encodeJSONPointerToken(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")

	return s
}
