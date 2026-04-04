package router

import (
	"encoding/json"
	"net/http"
	"regexp"

	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/state"
)

// paramPattern matches path parameter segments like {petId}.
var paramPattern = regexp.MustCompile(`\{(\w+)\}`)

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
	switch entry.Type {
	case model.BehaviorCreate:
		h.handleCreate(w, r, entry, gen, body)
	case model.BehaviorFetch:
		h.handleFetch(w, r, entry)
	case model.BehaviorList:
		h.handleList(w, entry)
	case model.BehaviorUpdate:
		h.handleUpdate(w, r, entry, body)
	case model.BehaviorDelete:
		h.handleDelete(w, r, entry)
	case model.BehaviorGeneric:
		return false
	default:
		return false
	}

	return true
}

// handleCreate generates a response body from the schema, stores the resource,
// and writes a 201 response.
func (h *Handler) handleCreate(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	gen *generator.DataGenerator,
	body []byte,
) {
	scenario := selectSuccess(&entry)
	seed := generator.Fingerprint(r.Method, r.URL.Path, r.URL.Query(), body)

	responseBody, ok := h.generateResponseBody(w, gen, scenario, seed)
	if !ok {
		return
	}

	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, responseBody)

	if err := h.store.Put(resourceType, resourceID, responseBody); err != nil {
		writeProblem(w, http.StatusInternalServerError, "failed to store resource: "+err.Error())

		return
	}

	writeJSON(w, scenario.StatusCode, responseBody)
}

// handleFetch looks up a resource by ID from the path and writes it as a
// 200 response, or writes a 404 if not found.
func (h *Handler) handleFetch(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil)

	data, found := h.store.Get(resourceType, resourceID)
	if !found {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	writeJSON(w, http.StatusOK, data)
}

// handleList returns all stored resources of the type derived from the path
// pattern. Returns an empty array if no resources exist.
func (h *Handler) handleList(
	w http.ResponseWriter,
	entry model.BehaviorEntry,
) {
	resourceType := state.ResourceType(entry.PathPattern)
	items := h.store.List(resourceType)

	writeJSON(w, http.StatusOK, items)
}

// handleUpdate merges the request body onto the stored resource and writes a
// 200 response. Both PUT and PATCH use shallow merge: request body fields
// overwrite stored fields, unmentioned fields are preserved. This matches
// developer expectations — a mock server should mimic real service behavior
// where PATCH updates specific fields and PUT replaces with the full
// representation (but preserves server-generated fields like id).
func (h *Handler) handleUpdate(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
	body []byte,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil)

	stored, found := h.store.Get(resourceType, resourceID)
	if !found {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	merged, err := shallowMerge(stored, body)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request body: "+err.Error())

		return
	}

	if putErr := h.store.Put(resourceType, resourceID, merged); putErr != nil {
		writeProblem(w, http.StatusInternalServerError, "failed to store resource: "+putErr.Error())

		return
	}

	writeJSON(w, http.StatusOK, merged)
}

// shallowMerge overlays parsed request body fields onto the stored resource.
// Top-level keys in the request body overwrite keys in the stored resource;
// keys not present in the request body are preserved from the stored resource.
func shallowMerge(stored any, reqBody []byte) (map[string]any, error) {
	storedMap, ok := stored.(map[string]any)
	if !ok {
		// Stored resource is not an object — replace entirely.
		storedMap = make(map[string]any)
	}

	if len(reqBody) == 0 {
		return storedMap, nil
	}

	var patch map[string]any
	if err := json.Unmarshal(reqBody, &patch); err != nil {
		return nil, err
	}

	for k, v := range patch {
		storedMap[k] = v
	}

	return storedMap, nil
}

// handleDelete removes a resource from the store and writes a 204 response,
// or a 404 if the resource does not exist.
func (h *Handler) handleDelete(
	w http.ResponseWriter,
	r *http.Request,
	entry model.BehaviorEntry,
) {
	pathParams := extractPathParams(r, entry.PathPattern)
	resourceType, resourceID := state.InferResourceIdentity(entry.PathPattern, pathParams, nil)

	if !h.store.Delete(resourceType, resourceID) {
		writeStatefulNotFound(w, resourceType, resourceID)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// extractPathParams reads path parameter values from the request using Go 1.22+
// ServeMux wildcards. Returns a map of param name → value.
func extractPathParams(r *http.Request, pathPattern string) map[string]string {
	matches := paramPattern.FindAllStringSubmatch(pathPattern, -1)
	params := make(map[string]string, len(matches))

	for _, m := range matches {
		name := m[1]
		if val := r.PathValue(name); val != "" {
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
