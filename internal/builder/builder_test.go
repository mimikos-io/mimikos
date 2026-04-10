package builder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/compiler"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	// expectedCorpus represents the expected classification JSON file format.
	expectedCorpus struct {
		Spec            string            `json:"spec"`
		Source          string            `json:"source"`
		TotalOperations int               `json:"total_operations"` //nolint:tagliatelle // matches corpus format
		Classifications map[string]string `json:"classifications"`
		Notes           map[string]string `json:"notes,omitempty"`
	}

	// misclassification records a single classification disagreement.
	misclassification struct {
		key      string
		expected string
		got      string
	}
)

func TestBuildBehaviorMap_NilSpec(t *testing.T) {
	_, err := BuildBehaviorMap(nil, classifier.New(), nil, nil)
	require.ErrorIs(t, err, ErrNilSpec)
}

func TestBuildBehaviorMap_NilClassifier(t *testing.T) {
	spec := &parser.ParsedSpec{}
	_, err := BuildBehaviorMap(spec, nil, nil, nil)
	require.ErrorIs(t, err, ErrNilClassifier)
}

func TestBuildBehaviorMap_EmptySpec(t *testing.T) {
	spec := &parser.ParsedSpec{}
	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, bm.Len())
}

func TestBuildBehaviorMap_ListOperation(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {StatusCode: http.StatusOK, Description: "OK"},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)
	require.Equal(t, 1, bm.Len())

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorList, entry.Type)
	assert.Equal(t, http.StatusOK, entry.SuccessCode)
	assert.Equal(t, model.SourceHeuristic, entry.Source)
}

func TestBuildBehaviorMap_CreateOperation(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodPost,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusCreated: {StatusCode: http.StatusCreated, Description: "Created"},
					http.StatusBadRequest: {
						StatusCode:  http.StatusBadRequest,
						Description: "Validation error",
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodPost, "/pets")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorCreate, entry.Type)
	assert.Equal(t, http.StatusCreated, entry.SuccessCode)
	// Error code 400 should be present as a key in ResponseSchemas (nil value = no schema).
	_, has400 := entry.ResponseSchemas[http.StatusBadRequest]
	assert.True(t, has400, "400 should be present in ResponseSchemas")
}

func TestBuildBehaviorMap_DeletePrefers204(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodDelete,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusOK:        {StatusCode: http.StatusOK, Description: "OK"},
					http.StatusNoContent: {StatusCode: http.StatusNoContent, Description: "Deleted"},
					http.StatusNotFound:  {StatusCode: http.StatusNotFound, Description: "Not found"},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodDelete, "/pets/{petId}")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorDelete, entry.Type)
	assert.Equal(t, http.StatusNoContent, entry.SuccessCode)
	// Error code 404 should be present as a key in ResponseSchemas (nil value = no schema).
	_, has404 := entry.ResponseSchemas[http.StatusNotFound]
	assert.True(t, has404, "404 should be present in ResponseSchemas")
}

func TestBuildBehaviorMap_NoResponsesDefaultsTo200(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				// No responses defined in spec.
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Equal(t, http.StatusOK, entry.SuccessCode)
}

func TestBuildBehaviorMap_OnlyErrorResponsesDefaultsTo200(t *testing.T) {
	// Spec defines only error responses, no 2xx at all.
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusNotFound:            {StatusCode: http.StatusNotFound},
					http.StatusInternalServerError: {StatusCode: http.StatusInternalServerError},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets/{petId}")
	require.True(t, ok)
	assert.Equal(t, http.StatusOK, entry.SuccessCode, "should default to 200 when no 2xx defined")
}

func TestBuildBehaviorMap_FallbackToLowest2xx(t *testing.T) {
	// Operation only defines 202, not 200. Should pick 202.
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusAccepted: {StatusCode: http.StatusAccepted, Description: "Accepted"},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Equal(t, http.StatusAccepted, entry.SuccessCode)
}

func TestBuildBehaviorMap_MultipleOperations(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{Method: http.MethodGet, Path: "/pets"},
			{Method: http.MethodPost, Path: "/pets"},
			{Method: http.MethodGet, Path: "/pets/{petId}"},
			{Method: http.MethodDelete, Path: "/pets/{petId}"},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, 4, bm.Len())

	listEntry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorList, listEntry.Type)

	createEntry, ok := bm.Get(http.MethodPost, "/pets")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorCreate, createEntry.Type)

	fetchEntry, ok := bm.Get(http.MethodGet, "/pets/{petId}")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorFetch, fetchEntry.Type)

	deleteEntry, ok := bm.Get(http.MethodDelete, "/pets/{petId}")
	require.True(t, ok)
	assert.Equal(t, model.BehaviorDelete, deleteEntry.Type)
}

func TestBuildBehaviorMap_NilCompilerSkipsSchemas(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				RequestBody: &parser.RequestBody{
					Required: true,
					Schema:   &parser.SchemaRef{Name: "PetInput", Pointer: "#/components/schemas/PetInput"},
				},
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Schema:     &parser.SchemaRef{Name: "PetList", Pointer: "#/components/schemas/PetList"},
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Nil(t, entry.RequestSchema, "request schema should be nil when compiler is nil")
	// ResponseSchemas has keys for defined status codes, but values are nil when compiler is nil.
	require.NotNil(t, entry.ResponseSchemas, "response schemas map should exist for defined status codes")
	assert.Nil(t, entry.ResponseSchemas[http.StatusOK], "schema value should be nil when compiler is nil")
}

func TestBuildBehaviorMap_SchemalessResponsesAsNilEntries(t *testing.T) {
	// Responses defined with no schema should appear as nil values in
	// ResponseSchemas, preserving "this status code exists" information.
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodPut,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusOK:                  {StatusCode: http.StatusOK},
					http.StatusBadRequest:          {StatusCode: http.StatusBadRequest},
					http.StatusNotFound:            {StatusCode: http.StatusNotFound},
					http.StatusInternalServerError: {StatusCode: http.StatusInternalServerError},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodPut, "/pets/{petId}")
	require.True(t, ok)

	// All four schema-less responses should be present as keys with nil values.
	for _, code := range []int{
		http.StatusOK, http.StatusBadRequest,
		http.StatusNotFound, http.StatusInternalServerError,
	} {
		schema, exists := entry.ResponseSchemas[code]
		assert.True(t, exists, "status %d should be present in ResponseSchemas", code)
		assert.Nil(t, schema, "status %d should have nil schema (no schema defined)", code)
	}
}

func TestBuildBehaviorMap_SchemalessDefaultResponseGetsKey0(t *testing.T) {
	// A default response with no schema should still appear as key 0
	// in ResponseSchemas with nil value, matching the status-code loop
	// behavior (Decision #62: key present = defined in spec).
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {StatusCode: http.StatusOK},
				},
				DefaultResponse: &parser.Response{
					StatusCode:  0,
					Description: "Error",
					// No Schema — description-only default.
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	require.NotNil(t, entry.ResponseSchemas)

	schema, exists := entry.ResponseSchemas[0]
	assert.True(t, exists, "default response should be present at key 0")
	assert.Nil(t, schema, "schema-less default should have nil value")
}

func TestBuildBehaviorMap_ConfidencePreserved(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{Method: http.MethodGet, Path: "/pets"},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Greater(t, entry.Confidence, 0.0, "confidence should be set by classifier")
	assert.LessOrEqual(t, entry.Confidence, 1.0)
}

func TestBuildBehaviorMap_WithCompiler(t *testing.T) {
	// Minimal valid OpenAPI 3.1 spec with a schema.
	specBytes := []byte(`{
		"openapi": "3.1.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/pets": {
				"get": {
					"operationId": "listPets",
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": { "$ref": "#/components/schemas/Pet" }
									}
								}
							}
						}
					}
				},
				"post": {
					"operationId": "createPet",
					"requestBody": {
						"required": true,
						"content": {
							"application/json": {
								"schema": { "$ref": "#/components/schemas/Pet" }
							}
						}
					},
					"responses": {
						"201": {
							"description": "Created",
							"content": {
								"application/json": {
									"schema": { "$ref": "#/components/schemas/Pet" }
								}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"Pet": {
					"type": "object",
					"properties": {
						"name": { "type": "string" }
					}
				}
			}
		}
	}`)

	sc, err := compiler.New(specBytes, "3.1.0")
	require.NoError(t, err)

	// Build a ParsedSpec that matches the spec structure.
	spec := &parser.ParsedSpec{
		Version: "3.1.0",
		Title:   "Test",
		Operations: []parser.Operation{
			{
				Method:      http.MethodGet,
				Path:        "/pets",
				OperationID: "listPets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Schema: &parser.SchemaRef{
							Name:    "listPets_200",
							Pointer: "#/paths/~1pets/get/responses/200/content/application~1json/schema",
						},
					},
				},
			},
			{
				Method:      http.MethodPost,
				Path:        "/pets",
				OperationID: "createPet",
				RequestBody: &parser.RequestBody{
					Required: true,
					Schema: &parser.SchemaRef{
						Name:    "Pet",
						Pointer: "#/components/schemas/Pet",
					},
				},
				Responses: map[int]*parser.Response{
					http.StatusCreated: {
						StatusCode: http.StatusCreated,
						Schema: &parser.SchemaRef{
							Name:    "Pet",
							Pointer: "#/components/schemas/Pet",
						},
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), sc, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, bm.Len())

	// GET /pets — response schema compiled.
	listEntry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	require.NotNil(t, listEntry.ResponseSchemas)
	assert.NotNil(t, listEntry.ResponseSchemas[http.StatusOK], "200 response schema should be compiled")
	assert.Nil(t, listEntry.RequestSchema, "GET has no request body")

	// POST /pets — both request and response schemas compiled.
	createEntry, ok := bm.Get(http.MethodPost, "/pets")
	require.True(t, ok)
	assert.NotNil(t, createEntry.RequestSchema, "POST request schema should be compiled")
	require.NotNil(t, createEntry.ResponseSchemas)
	assert.NotNil(t, createEntry.ResponseSchemas[http.StatusCreated], "201 response schema should be compiled")
}

func TestBuildBehaviorMap_DefaultResponseWithCompiler(t *testing.T) {
	specBytes := []byte(`{
		"openapi": "3.1.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {
			"/pets": {
				"get": {
					"operationId": "listPets",
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": { "type": "array", "items": { "type": "string" } }
								}
							}
						},
						"default": {
							"description": "Error",
							"content": {
								"application/json": {
									"schema": { "$ref": "#/components/schemas/Error" }
								}
							}
						}
					}
				}
			}
		},
		"components": {
			"schemas": {
				"Error": {
					"type": "object",
					"properties": {
						"message": { "type": "string" }
					}
				}
			}
		}
	}`)

	sc, err := compiler.New(specBytes, "3.1.0")
	require.NoError(t, err)

	spec := &parser.ParsedSpec{
		Version: "3.1.0",
		Operations: []parser.Operation{
			{
				Method:      http.MethodGet,
				Path:        "/pets",
				OperationID: "listPets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Schema: &parser.SchemaRef{
							Name:    "listPets_200",
							Pointer: "#/paths/~1pets/get/responses/200/content/application~1json/schema",
						},
					},
				},
				DefaultResponse: &parser.Response{
					StatusCode:  0,
					Description: "Error",
					Schema: &parser.SchemaRef{
						Name:    "Error",
						Pointer: "#/components/schemas/Error",
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), sc, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	require.NotNil(t, entry.ResponseSchemas)
	assert.NotNil(t, entry.ResponseSchemas[http.StatusOK], "200 response schema should be compiled")
	assert.NotNil(t, entry.ResponseSchemas[0], "default response schema should be at key 0")
}

func TestBuildBehaviorMap_SchemaCompilationFailureIsResilient(t *testing.T) {
	// Spec with a valid schema for one operation and an invalid pointer for another.
	specBytes := []byte(`{
		"openapi": "3.1.0",
		"info": {"title": "Test", "version": "1.0"},
		"paths": {},
		"components": {
			"schemas": {
				"Pet": {
					"type": "object",
					"properties": { "name": { "type": "string" } }
				}
			}
		}
	}`)

	sc, err := compiler.New(specBytes, "3.1.0")
	require.NoError(t, err)

	spec := &parser.ParsedSpec{
		Version: "3.1.0",
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Schema: &parser.SchemaRef{
							Name:    "Pet",
							Pointer: "#/components/schemas/Pet",
						},
					},
				},
			},
			{
				Method: http.MethodGet,
				Path:   "/dogs",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Schema: &parser.SchemaRef{
							Name:    "DoesNotExist",
							Pointer: "#/components/schemas/DoesNotExist",
						},
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), sc, nil)
	require.NoError(t, err, "should not fail on individual schema compilation error")
	assert.Equal(t, 2, bm.Len(), "both operations should be in the map")

	// First operation has a valid schema.
	petEntry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	require.NotNil(t, petEntry.ResponseSchemas)
	assert.NotNil(t, petEntry.ResponseSchemas[http.StatusOK])

	// Second operation has a failed schema — status code present, schema value nil.
	dogEntry, ok := bm.Get(http.MethodGet, "/dogs")
	require.True(t, ok)
	require.NotNil(t, dogEntry.ResponseSchemas, "response schemas map should exist for defined status codes")
	assert.Nil(t, dogEntry.ResponseSchemas[http.StatusOK], "failed schema compilation should result in nil value")
}

func TestBuildBehaviorMap_OperationIDPreserved(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method:      http.MethodGet,
				Path:        "/pets",
				OperationID: "listPets",
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Equal(t, "listPets", entry.OperationID)
}

// --- Response Examples ---

func TestBuildBehaviorMap_ResponseExamplesThreaded(t *testing.T) {
	exampleData := map[string]any{"id": 1, "name": "Fido"}

	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						Example:    exampleData,
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets/{petId}")
	require.True(t, ok)
	require.NotNil(t, entry.ResponseExamples, "ResponseExamples should be set when examples exist")

	ex, exists := entry.ResponseExamples[http.StatusOK]
	assert.True(t, exists, "200 example should be present")
	assert.Equal(t, exampleData, ex)
}

func TestBuildBehaviorMap_ResponseExamplesNoExample(t *testing.T) {
	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets",
				Responses: map[int]*parser.Response{
					http.StatusOK: {
						StatusCode: http.StatusOK,
						// No Example set.
					},
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets")
	require.True(t, ok)
	assert.Nil(t, entry.ResponseExamples, "ResponseExamples should be nil when no examples exist")
}

func TestBuildBehaviorMap_ResponseExamplesDefaultResponse(t *testing.T) {
	errorExample := map[string]any{"message": "not found"}

	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusOK: {StatusCode: http.StatusOK},
				},
				DefaultResponse: &parser.Response{
					StatusCode: 0,
					Example:    errorExample,
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets/{petId}")
	require.True(t, ok)
	require.NotNil(t, entry.ResponseExamples)

	ex, exists := entry.ResponseExamples[0]
	assert.True(t, exists, "default response example should be at key 0")
	assert.Equal(t, errorExample, ex)
}

func TestBuildBehaviorMap_ResponseExamplesMixedCodes(t *testing.T) {
	okExample := map[string]any{"id": 1}

	spec := &parser.ParsedSpec{
		Operations: []parser.Operation{
			{
				Method: http.MethodGet,
				Path:   "/pets/{petId}",
				Responses: map[int]*parser.Response{
					http.StatusOK:       {StatusCode: http.StatusOK, Example: okExample},
					http.StatusNotFound: {StatusCode: http.StatusNotFound}, // No example.
				},
			},
		},
	}

	bm, err := BuildBehaviorMap(spec, classifier.New(), nil, nil)
	require.NoError(t, err)

	entry, ok := bm.Get(http.MethodGet, "/pets/{petId}")
	require.True(t, ok)
	require.NotNil(t, entry.ResponseExamples)

	// 200 has example.
	ex, exists := entry.ResponseExamples[http.StatusOK]
	assert.True(t, exists)
	assert.Equal(t, okExample, ex)

	// 404 should NOT be in ResponseExamples (no example defined).
	_, exists = entry.ResponseExamples[http.StatusNotFound]
	assert.False(t, exists, "404 without example should not be in ResponseExamples")
}

// --- Corpus-driven integration test ---
// Tests the full pipeline: spec bytes → parser → compiler → builder → BehaviorMap.
// Also validates schema compilation against real-world specs.

// testdataDir returns the absolute path to the testdata directory root.
func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

// TestCorpusBuildBehaviorMap tests the full pipeline from spec bytes through
// BuildBehaviorMap, verifying both classification accuracy and that schema
// compilation succeeds on real-world specs.
func TestCorpusBuildBehaviorMap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping corpus integration test in short mode")
	}

	base := testdataDir(t)
	expectedDir := filepath.Join(base, "expected")
	specsDir := filepath.Join(base, "specs")

	expectedFiles, err := filepath.Glob(filepath.Join(expectedDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, expectedFiles, "no expected classification files found")

	p := parser.NewLibopenAPIParser(nil)
	cls := classifier.New()

	var (
		totalCorrect       int
		totalChecked       int
		totalSchemaCompile int
		specsTested        int
		specsSkipped       int
		allMisses          []misclassification
	)

	for _, expectedFile := range expectedFiles {
		var corpus expectedCorpus

		data, readErr := os.ReadFile(expectedFile)
		require.NoError(t, readErr)
		require.NoError(t, json.Unmarshal(data, &corpus))

		specName := corpus.Spec
		specPath := filepath.Join(specsDir, specName)

		if _, statErr := os.Stat(specPath); os.IsNotExist(statErr) {
			t.Logf("SKIP %s (spec file not present)", specName)

			specsSkipped++

			continue
		}

		specsTested++

		specData, readErr := os.ReadFile(specPath)
		require.NoError(t, readErr, "reading spec %s", specName)

		// Full pipeline: document → parse → compile → build.
		doc, docErr := libopenapi.NewDocument(specData)
		require.NoError(t, docErr, "creating document for %s", specName)

		spec, parseErr := p.Parse(context.Background(), doc)
		require.NoError(t, parseErr, "parsing spec %s", specName)

		sc, compileErr := compiler.New(specData, spec.Version)
		require.NoError(t, compileErr, "creating compiler for %s", specName)

		bm, buildErr := BuildBehaviorMap(spec, cls, sc, nil)
		require.NoError(t, buildErr, "building behavior map for %s", specName)

		var (
			correct     int
			checked     int
			schemaCount int
		)

		for _, entry := range bm.Entries() {
			key := entry.Method + " " + entry.PathPattern
			expectedType, ok := corpus.Classifications[key]

			if !ok {
				continue
			}

			checked++

			if entry.Type.String() == expectedType {
				correct++
			} else {
				allMisses = append(allMisses, misclassification{
					key:      key,
					expected: expectedType,
					got:      entry.Type.String(),
				})
			}

			// Count compiled schemas (response + request).
			for _, cs := range entry.ResponseSchemas {
				if cs != nil {
					schemaCount++
				}
			}

			if entry.RequestSchema != nil {
				schemaCount++
			}
		}

		accuracy := float64(0)
		if checked > 0 {
			accuracy = float64(correct) / float64(checked) * 100
		}

		t.Logf("%-30s %3d/%3d correct (%.1f%%)  schemas: %d compiled",
			specName, correct, checked, accuracy, schemaCount)

		totalCorrect += correct
		totalChecked += checked
		totalSchemaCompile += schemaCount
	}

	// Overall summary.
	overallAccuracy := float64(0)
	if totalChecked > 0 {
		overallAccuracy = float64(totalCorrect) / float64(totalChecked) * 100
	}

	t.Logf("")
	t.Logf("SPECS: %d tested, %d skipped", specsTested, specsSkipped)
	t.Logf("OVERALL: %d/%d correct (%.1f%%)", totalCorrect, totalChecked, overallAccuracy)
	t.Logf("SCHEMAS: %d compiled", totalSchemaCompile)
	t.Logf("MISCLASSIFICATIONS: %d", len(allMisses))

	if len(allMisses) > 0 {
		sort.Slice(allMisses, func(i, j int) bool {
			if allMisses[i].expected != allMisses[j].expected {
				return allMisses[i].expected < allMisses[j].expected
			}

			if allMisses[i].got != allMisses[j].got {
				return allMisses[i].got < allMisses[j].got
			}

			return allMisses[i].key < allMisses[j].key
		})

		categories := make(map[string]int)

		for _, m := range allMisses {
			cat := fmt.Sprintf("%s → %s", m.expected, m.got)
			categories[cat]++
		}

		t.Logf("")
		t.Logf("Misclassification categories:")

		catKeys := make([]string, 0, len(categories))
		for k := range categories {
			catKeys = append(catKeys, k)
		}

		sort.Strings(catKeys)

		for _, cat := range catKeys {
			t.Logf("  %-25s %d", cat, categories[cat])
		}
	}

	assert.GreaterOrEqual(t, specsTested, 5,
		"at least 5 specs must be present for meaningful accuracy numbers")
	assert.Greater(t, totalChecked, 50,
		"expected to check at least 50 operations")

	// Accuracy regression guard — Session 13 exit criteria requires >90%.
	assert.GreaterOrEqual(t, overallAccuracy, 90.0,
		"corpus accuracy must be >= 90%% (got %.1f%%)", overallAccuracy)

	t.Logf("")
	t.Logf("CORPUS_ACCURACY=%.1f CHECKED=%d CORRECT=%d MISCLASSIFIED=%d SPECS=%d SCHEMAS=%d",
		overallAccuracy, totalChecked, totalCorrect, len(allMisses),
		specsTested, totalSchemaCompile)
}
