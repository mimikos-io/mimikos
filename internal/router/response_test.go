package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	merrors "github.com/mimikos-io/mimikos/internal/errors"
	"github.com/mimikos-io/mimikos/internal/generator"
	"github.com/mimikos-io/mimikos/internal/model"
)

// compileTestSchema compiles a JSON Schema string into a *jsonschema.Schema
// for use in response validation tests. AddResource expects unmarshaled data.
func compileTestSchema(t *testing.T, schemaJSON string) *jsonschema.Schema {
	t.Helper()

	var doc any
	require.NoError(t, json.Unmarshal([]byte(schemaJSON), &doc))

	c := jsonschema.NewCompiler()
	require.NoError(t, c.AddResource("test.json", doc))

	sch, err := c.Compile("test.json")
	require.NoError(t, err)

	return sch
}

// behaviorMapWithSchema builds a single-entry BehaviorMap using a compiled schema.
func behaviorMapWithSchema(sch *jsonschema.Schema) *model.BehaviorMap {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/items/{id}",
		Type:        model.BehaviorFetch,

		SuccessCode: http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{
			http.StatusOK: {Name: "Item", Schema: sch},
		},
		Source:     model.SourceHeuristic,
		Confidence: 0.9,
	})

	return bm
}

func TestResponseValidation_ValidResponse(t *testing.T) {
	// Schema that the generator will satisfy: simple object with integer + string.
	sch := compileTestSchema(t, `{
		"type": "object",
		"properties": {
			"id":   {"type": "integer"},
			"name": {"type": "string"}
		}
	}`)

	bm := behaviorMapWithSchema(sch)
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)
	h := NewHandler(bm, &stubValidator{}, resp, gen, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/items/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))
}

func TestResponseValidation_InvalidDefault_SendsAnyway(t *testing.T) {
	// Schema with maxLength: 5 on "email" field. The semantic mapper produces
	// real email addresses (~20+ chars), which violates maxLength. This is a
	// known design choice (Decision #41: semantic values bypass constraints).
	sch := compileTestSchema(t, `{
		"type": "object",
		"properties": {
			"email": {"type": "string", "maxLength": 5}
		},
		"required": ["email"]
	}`)

	bm := behaviorMapWithSchema(sch)
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)
	h := NewHandler(bm, &stubValidator{}, resp, gen, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/items/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Default mode: response is sent despite validation failure.
	assert.Equal(t, http.StatusOK, rec.Code)
	//nolint:testifylint // header string, not JSON
	assert.Equal(t, contentTypeJSON, rec.Header().Get("Content-Type"))

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body, "email")
}

func TestResponseValidation_InvalidStrict_Returns500(t *testing.T) {
	// Same schema that reliably triggers validation failure.
	sch := compileTestSchema(t, `{
		"type": "object",
		"properties": {
			"email": {"type": "string", "maxLength": 5}
		},
		"required": ["email"]
	}`)

	bm := behaviorMapWithSchema(sch)
	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)
	h := NewHandler(bm, &stubValidator{}, resp, gen, true, nil)

	req := httptest.NewRequest(http.MethodGet, "/items/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	pd := parseProblemDetail(t, rec.Body.Bytes())
	assert.Equal(t, "Internal Server Error", pd["title"])
	assert.Contains(t, pd["detail"], "response validation failed")
}

func TestResponseValidation_NilSchema_SkipsValidation(t *testing.T) {
	// Entry with no response schema — validation should be skipped.
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodGet,
		PathPattern: "/empty",
		Type:        model.BehaviorFetch,

		SuccessCode:     http.StatusOK,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)
	h := NewHandler(bm, &stubValidator{}, resp, gen, true, nil)

	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Even in strict mode, nil schema should not cause 500.
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestResponseValidation_204NoContent_SkipsValidation(t *testing.T) {
	bm := model.NewBehaviorMap()
	bm.Put(model.BehaviorEntry{
		Method:      http.MethodDelete,
		PathPattern: "/items/{id}",
		Type:        model.BehaviorDelete,

		SuccessCode:     http.StatusNoContent,
		ResponseSchemas: map[int]*model.CompiledSchema{},
		Source:          model.SourceHeuristic,
		Confidence:      0.9,
	})

	resp := merrors.NewResponder()
	gen := generator.NewDataGenerator(generator.NewSemanticMapper(), 0)
	h := NewHandler(bm, &stubValidator{}, resp, gen, true, nil)

	req := httptest.NewRequest(http.MethodDelete, "/items/1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Empty(t, rec.Body.Bytes())
}
