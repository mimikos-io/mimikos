package router

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
)

// paramPattern matches path parameter segments like {petId} or {enterprise-team}.
// Uses [^}]+ instead of \w+ to support hyphens and dots in OpenAPI parameter names.
var paramPattern = regexp.MustCompile(`\{([^}]+)\}`)

// handleStatefulMode processes a request in stateful mode. Returns true if the
// request was handled (response written), false if it should fall through
// to deterministic logic (generic behavior type). The body parameter is the
// pre-read request body from validateRequest — r.Body is exhausted at this
// point, so callers must pass the body explicitly.
func (h *Handler) handleStatefulMode(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	body []byte,
) bool {
	// Response schema degraded: behaviors that generate from schema (create,
	// list, non-204 delete) cannot function without a compiled schema. Block
	// with RFC 7807 — unless a media-type example provides a fallback.
	if entry.DegradedResponseSchema != "" {
		if isDegradedStateful(entry) {
			writeProblem(w, http.StatusInternalServerError,
				"Response generation unavailable — schema failed to compile at startup: "+
					entry.DegradedResponseSchema)

			return true
		}
	}

	switch entry.Type {
	case model.BehaviorCreate:
		h.handleCreate(w, r, entry, gen, body)
	case model.BehaviorFetch:
		h.handleFetch(w, r, entry)
	case model.BehaviorList:
		h.handleList(w, r, entry, gen, body)
	case model.BehaviorUpdate:
		h.handleUpdate(w, r, entry, body)
	case model.BehaviorDelete:
		h.handleDelete(w, r, entry, gen, body)
	case model.BehaviorGeneric:
		return false
	default:
		return false
	}

	return true
}

// handleCreate builds a response using a three-layer merge pipeline, stores
// the unwrapped resource, and writes the (potentially wrapped) response.
//
// The layers are applied in priority order (last wins):
//  1. Faker base — generate all fields from schema (provides unique IDs)
//  2. Example overlay — realistic defaults from spec author (ID-protected)
//  3. Request body overlay — user intent wins over everything
func (h *Handler) handleCreate(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	body []byte,
) {
	scenario := selectSuccess(&entry)
	seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)

	// Layer 1: faker base (or empty map if no schema).
	fakerBase, ok := h.generateResponseBody(w, gen, scenario, seed)
	if !ok {
		return
	}

	merged := fakerBase

	// Layer 2: example overlay with ID protection.
	// When the schema is degraded (nil), faker produced an empty map — protecting
	// ID fields would strip the example's ID leaving no ID at all. A static
	// example ID is better than none when faker can't generate one.
	if scenario.Example != nil {
		exampleFields := exampleAsMap(scenario.Example, entry.WrapperKey)
		if baseMap, ok := unwrapResource(fakerBase, entry.WrapperKey).(map[string]any); ok && len(exampleFields) > 0 {
			if scenario.Schema == nil || scenario.Schema.Schema == nil {
				merged = wrapResource(shallowMerge(baseMap, exampleFields), entry.WrapperKey)
			} else {
				idFields := buildIDFieldSet(entry)
				merged = wrapResource(shallowMergeExcluding(baseMap, exampleFields, idFields), entry.WrapperKey)
			}
		}
	}

	// Layer 3: request body overlay (no exclusions — user intent wins).
	reqFields, err := parseRequestBody(body, entry.WrapperKey)
	if err == nil && len(reqFields) > 0 {
		if resourceMap, ok := unwrapResource(merged, entry.WrapperKey).(map[string]any); ok {
			merged = wrapResource(shallowMerge(resourceMap, reqFields), entry.WrapperKey)
		}
	}

	// Unwrap for storage and identity extraction.
	resource := unwrapResource(merged, entry.WrapperKey)

	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, resource, entry.IDFieldHint)
	scope := state.ParentScope(entry.PathPattern, pathParams)

	if err := h.store.Put(resourceType, scope, resourceID, resource); err != nil {
		writeProblem(w, http.StatusInternalServerError, "failed to store resource: "+err.Error())

		return
	}

	writeJSON(w, scenario.StatusCode, merged)
}

// exampleAsMap extracts the inner resource fields from a media-type example.
// If the example is a wrapped object (e.g., {"data": {...}}), it unwraps
// through the wrapper key and returns the inner map. Returns nil if the
// example is not a map or the inner value is not a map.
func exampleAsMap(example any, wrapperKey string) map[string]any {
	inner := unwrapResource(example, wrapperKey)

	m, ok := inner.(map[string]any)
	if !ok {
		return nil
	}

	return m
}

// handleFetch looks up a resource by ID from the path, re-wraps it if needed,
// and writes it as a 200 response. Writes 404 if not found.
func (h *Handler) handleFetch(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil, entry.IDFieldHint)
	scope := state.ParentScope(entry.PathPattern, pathParams)

	data, found := h.store.Get(resourceType, scope, resourceID)
	if !found {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	writeJSON(w, entry.SuccessCode, wrapResource(data, entry.WrapperKey))
}

// handleList returns all stored resources of the type derived from the path
// pattern. For bare-array list schemas, returns a JSON array directly.
// For object-wrapped list schemas, generates the envelope from the schema
// (pagination metadata, etc.) and replaces the array slot with stored resources.
func (h *Handler) handleList(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	body []byte,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType := state.ResourceType(entry.PathPattern)
	scope := state.ParentScope(entry.PathPattern, pathParams)
	items := h.store.List(resourceType, scope)

	if entry.ListArrayKey == "" {
		// Bare array (Petstore, GitHub).
		writeJSON(w, entry.SuccessCode, items)

		return
	}

	// Object-wrapped list: generate the full envelope from schema
	// (produces pagination metadata, type discriminators, etc.),
	// then replace the array slot with actual stored resources.
	scenario := selectSuccess(&entry)
	seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)
	envelope := h.generateListEnvelope(gen, scenario, seed)
	envelope[entry.ListArrayKey] = items

	writeJSON(w, entry.SuccessCode, envelope)
}

// generateListEnvelope produces the list response skeleton from the schema.
// Returns a map with all non-array fields populated from the generator
// (pagination metadata, type discriminators). The array slot will be
// overwritten by the caller with actual stored resources.
func (h *Handler) generateListEnvelope(
	gen *generator.DataGenerator,
	scenario *SelectedScenario,
	seed int64,
) map[string]any {
	if scenario.Schema == nil || scenario.Schema.Schema == nil {
		return make(map[string]any)
	}

	body, err := gen.Generate(scenario.Schema.Schema, seed)
	if err != nil {
		h.logger.Warn("failed to generate list envelope from schema",
			"schema", scenario.Schema.Name,
			"error", err.Error(),
		)

		return make(map[string]any)
	}

	m, ok := body.(map[string]any)
	if !ok {
		h.logger.Warn("list envelope generation produced non-object",
			"schema", scenario.Schema.Name,
		)

		return make(map[string]any)
	}

	return m
}

// handleUpdate parses and optionally unwraps the request body, merges it onto
// the stored resource (cloned to avoid mutation), re-wraps if needed, and writes
// a 200 response.
func (h *Handler) handleUpdate(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	body []byte,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil, entry.IDFieldHint)
	scope := state.ParentScope(entry.PathPattern, pathParams)

	stored, found := h.store.Get(resourceType, scope, resourceID)
	if !found {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	// Parse and optionally unwrap request body. This is the single place
	// where malformed JSON produces a 400 error.
	patch, err := parseRequestBody(body, entry.WrapperKey)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request body: "+err.Error())

		return
	}

	merged := shallowMerge(stored, patch)

	if putErr := h.store.Put(resourceType, scope, resourceID, merged); putErr != nil {
		writeProblem(w, http.StatusInternalServerError, "failed to store resource: "+putErr.Error())

		return
	}

	writeJSON(w, entry.SuccessCode, wrapResource(merged, entry.WrapperKey))
}

// shallowMerge clones the stored resource and overlays patch fields.
// Top-level keys in patch overwrite stored keys; keys not in patch are
// preserved. Always returns a new map — never mutates the input.
func shallowMerge(stored any, patch map[string]any) map[string]any {
	storedMap, ok := stored.(map[string]any)
	if !ok {
		storedMap = make(map[string]any)
	}

	// Clone to avoid mutating the store's reference.
	clone := make(map[string]any, len(storedMap)+len(patch))
	for k, v := range storedMap {
		clone[k] = v
	}

	for k, v := range patch {
		clone[k] = v
	}

	return clone
}

// handleDelete removes a resource from the store and writes the spec-defined
// success response. Most specs return 204 No Content; some (Asana) return 200
// with a response body. Writes 404 if the resource does not exist.
func (h *Handler) handleDelete(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	body []byte,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil, entry.IDFieldHint)
	scope := state.ParentScope(entry.PathPattern, pathParams)

	if !h.store.Delete(resourceType, scope, resourceID) {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	if entry.SuccessCode == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)

		return
	}

	// Non-204 delete (e.g., Asana returns 200 with {data: {}}).
	// Generate the response body from the schema.
	scenario := selectSuccess(&entry)
	seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)

	responseBody, ok := h.generateResponseBody(w, gen, scenario, seed)
	if !ok {
		return
	}

	writeJSON(w, entry.SuccessCode, responseBody)
}

// isDegradedStateful returns true if a stateful behavior cannot function due to
// response schema degradation. Create and list always need a schema for
// generation. Delete needs one only for non-204 responses. Fetch and update
// use stored data, not the schema, so they are never degraded.
func isDegradedStateful(entry model.BehaviorEntry) bool {
	scenario := selectSuccess(&entry)

	// Media-type example available — can serve without a schema.
	if scenario.Example != nil {
		return false
	}

	switch entry.Type {
	case model.BehaviorCreate, model.BehaviorList:
		return true
	case model.BehaviorDelete:
		return entry.SuccessCode != http.StatusNoContent
	case model.BehaviorFetch, model.BehaviorUpdate, model.BehaviorGeneric:
		// Fetch/update use stored data, not the schema. Generic falls through
		// to deterministic mode. None are degraded by schema compilation failure.
		return false
	}

	return false
}

// unwrapResource extracts the inner resource from a potentially wrapped body.
// If wrapperKey is empty (flat response), returns body unchanged.
// If wrapperKey is set, returns body[wrapperKey] if it's a map, or body unchanged.
func unwrapResource(body any, wrapperKey string) any {
	if wrapperKey == "" {
		return body
	}

	m, ok := body.(map[string]any)
	if !ok {
		return body
	}

	inner, exists := m[wrapperKey]
	if !exists {
		return body
	}

	return inner
}

// wrapResource wraps a resource in a single-key envelope.
// If wrapperKey is empty (flat response), returns resource unchanged.
func wrapResource(resource any, wrapperKey string) any {
	if wrapperKey == "" {
		return resource
	}

	return map[string]any{wrapperKey: resource}
}

// parseRequestBody unmarshals the request body JSON and optionally unwraps
// it through the wrapper key. Ownership of JSON parsing is here, not in
// shallowMerge — this function is the single place where malformed JSON
// produces a parse error.
//
// When wrapperKey is "": parses body into map[string]any.
// When wrapperKey is "data": parses body, extracts body["data"] as map[string]any.
// Returns empty map (not nil) for empty body.
func parseRequestBody(body []byte, wrapperKey string) (map[string]any, error) {
	if len(body) == 0 {
		return make(map[string]any), nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	if wrapperKey == "" {
		return parsed, nil
	}

	// Unwrap: extract inner object from wrapper key.
	inner, exists := parsed[wrapperKey]
	if !exists {
		// No wrapper key in body — pass through the full parsed body.
		// The client may have sent an unwrapped PATCH even for a wrapped spec.
		return parsed, nil
	}

	innerMap, ok := inner.(map[string]any)
	if !ok {
		return parsed, nil
	}

	return innerMap, nil
}

// buildIDFieldSet computes the set of field names that should be protected
// from example overwrite during create. Mirrors strategies 1–4 of
// [state.InferResourceIdentity] to determine which body fields hold IDs.
// Strategies 5–6 (path param value, UUID fallback) are about reading IDs,
// not field names, so they don't apply here.
// All keys are stored lowercase for case-insensitive comparison.
func buildIDFieldSet(entry model.BehaviorEntry) map[string]bool {
	set := map[string]bool{"id": true} // Strategy 4: always protected.

	lastParam := state.LastPathParam(entry.PathPattern)
	if lastParam != "" {
		set[strings.ToLower(lastParam)] = true // Strategy 1: exact match.

		// Strategy 2: suffix strip.
		leaf := state.LeafCollection(entry.PathPattern)
		if remainder := state.StripResourcePrefix(lastParam, leaf); remainder != "" {
			set[strings.ToLower(remainder)] = true
		}
	}

	if entry.IDFieldHint != "" {
		set[strings.ToLower(entry.IDFieldHint)] = true // Strategy 3: precomputed hint.
	}

	return set
}

// shallowMergeExcluding merges overlay onto base, skipping overlay keys that
// appear in the exclude set (case-insensitive). Returns a new map — neither
// base nor overlay is mutated. Used to merge example fields onto faker output
// while protecting ID fields from static example values.
func shallowMergeExcluding(base, overlay map[string]any, exclude map[string]bool) map[string]any {
	clone := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		clone[k] = v
	}

	for k, v := range overlay {
		if exclude[strings.ToLower(k)] {
			continue
		}

		clone[k] = v
	}

	return clone
}

// extractPathParams reads path parameter values from the request using chi's
// URL parameter extraction. Returns a map of param name → value.
func extractPathParams(r *http.Request, pathPattern string) map[string]string {
	matches := paramPattern.FindAllStringSubmatch(pathPattern, -1)
	params := make(map[string]string, len(matches))

	for _, m := range matches {
		name := m[1]
		if val := chi.URLParam(r, name); val != "" {
			params[name] = val
		}
	}

	return params
}

// writeStatefulNotFound writes an RFC 7807 404 response for a missing resource.
func writeStatefulNotFound(w http.ResponseWriter, resourceType, resourceID string) {
	writeProblem(w, http.StatusNotFound,
		"Resource "+resourceType+"/"+resourceID+" not found")
}

// writeJSON encodes body as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)

	//nolint:errchkjson // write failures (client disconnect) are unrecoverable
	_ = json.NewEncoder(w).Encode(body)
}
