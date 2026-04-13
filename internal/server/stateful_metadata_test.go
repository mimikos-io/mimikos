package server

import (
	"log/slog"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mimikos-io/mimikos/internal/model"
)

// newTypes creates a *jsonschema.Types with the given type strings.
func newTypes(types ...string) *jsonschema.Types {
	var t jsonschema.Types
	for _, typ := range types {
		t.Add(typ)
	}

	return &t
}

// compiledSchema wraps a *jsonschema.Schema in a *model.CompiledSchema.
func compiledSchema(s *jsonschema.Schema) *model.CompiledSchema {
	return &model.CompiledSchema{Schema: s}
}

// --- detectWrapperKey tests ---

func TestDetectWrapperKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		schemas     map[int]*model.CompiledSchema
		successCode int
		want        string
	}{
		{
			name:        "nil schema returns empty",
			schemas:     nil,
			successCode: 200,
			want:        "",
		},
		{
			name: "nil compiled schema returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: nil,
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "flat object with multiple properties returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"id":   {Types: newTypes("string")},
						"name": {Types: newTypes("string")},
					},
				}),
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "single-key object wrapper returns key",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"data": {Types: newTypes("object")},
					},
				}),
			},
			successCode: 200,
			want:        "data",
		},
		{
			name: "single-key via $ref returns key",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"data": {Ref: &jsonschema.Schema{Types: newTypes("object")}},
					},
				}),
			},
			successCode: 200,
			want:        "data",
		},
		{
			name: "single-key non-object returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"count": {Types: newTypes("integer")},
					},
				}),
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "single-key with additionalProperties true returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"data": {Types: newTypes("object")},
					},
					AdditionalProperties: true,
				}),
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "falls back to default response (key 0) when success code missing",
			schemas: map[int]*model.CompiledSchema{
				0: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"data": {Types: newTypes("object")},
					},
				}),
			},
			successCode: 201,
			want:        "data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detectWrapperKey(tt.schemas, tt.successCode)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- detectListArrayKey tests ---

func TestDetectListArrayKey(t *testing.T) {
	t.Parallel()

	logger := slog.Default()

	tests := []struct {
		name        string
		schemas     map[int]*model.CompiledSchema
		successCode int
		want        string
	}{
		{
			name:        "nil schema returns empty",
			schemas:     nil,
			successCode: 200,
			want:        "",
		},
		{
			name: "bare array returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("array"),
				}),
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "object with one array property returns key",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"data":     {Types: newTypes("array")},
						"has_more": {Types: newTypes("boolean")},
					},
				}),
			},
			successCode: 200,
			want:        "data",
		},
		{
			name: "array property via $ref returns key",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"items": {Ref: &jsonschema.Schema{Types: newTypes("array")}},
					},
				}),
			},
			successCode: 200,
			want:        "items",
		},
		{
			name: "multiple array properties returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"items":  {Types: newTypes("array")},
						"errors": {Types: newTypes("array")},
					},
				}),
			},
			successCode: 200,
			want:        "",
		},
		{
			name: "object with no array properties returns empty",
			schemas: map[int]*model.CompiledSchema{
				200: compiledSchema(&jsonschema.Schema{
					Types: newTypes("object"),
					Properties: map[string]*jsonschema.Schema{
						"count": {Types: newTypes("integer")},
					},
				}),
			},
			successCode: 200,
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := detectListArrayKey(tt.schemas, tt.successCode, logger)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- computeIDFieldHint tests ---

func TestComputeIDFieldHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		entry           model.BehaviorEntry
		paramByResource map[string]string
		want            string
	}{
		{
			name: "fetch param with suffix produces hint",
			entry: model.BehaviorEntry{
				PathPattern: "/projects",
				Type:        model.BehaviorCreate,
			},
			paramByResource: map[string]string{
				"projects": "project_gid",
			},
			want: "gid",
		},
		{
			name: "fetch param without suffix produces empty hint",
			entry: model.BehaviorEntry{
				PathPattern: "/items",
				Type:        model.BehaviorCreate,
			},
			paramByResource: map[string]string{
				"items": "id",
			},
			want: "",
		},
		{
			name: "no fetch entry produces empty hint",
			entry: model.BehaviorEntry{
				PathPattern: "/orphans",
				Type:        model.BehaviorCreate,
			},
			paramByResource: map[string]string{},
			want:            "",
		},
		{
			name: "camelCase suffix produces hint",
			entry: model.BehaviorEntry{
				PathPattern: "/pets",
				Type:        model.BehaviorCreate,
			},
			paramByResource: map[string]string{
				"pets": "petId",
			},
			want: "id",
		},
		{
			name: "nested path uses namespace key for lookup and leaf for strip",
			entry: model.BehaviorEntry{
				PathPattern: "/projects/{project_gid}/sections",
				Type:        model.BehaviorCreate,
			},
			paramByResource: map[string]string{
				"projects/*/sections": "section_gid",
			},
			want: "gid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := computeIDFieldHint(tt.entry, tt.paramByResource)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- buildParamIndex tests ---

func TestBuildParamIndex(t *testing.T) {
	t.Parallel()

	entries := []model.BehaviorEntry{
		{Method: "POST", PathPattern: "/projects", Type: model.BehaviorCreate},
		{Method: "GET", PathPattern: "/projects/{project_gid}", Type: model.BehaviorFetch},
		{Method: "GET", PathPattern: "/projects", Type: model.BehaviorList},
		{Method: "PUT", PathPattern: "/tasks/{task_gid}", Type: model.BehaviorUpdate},
		{Method: "DELETE", PathPattern: "/pets/{petId}", Type: model.BehaviorDelete},
		// Nested path: namespace includes parent hierarchy.
		{Method: "GET", PathPattern: "/projects/{project_gid}/sections/{section_gid}", Type: model.BehaviorFetch},
	}

	idx := buildParamIndex(entries)

	assert.Equal(t, "project_gid", idx["projects"])
	assert.Equal(t, "task_gid", idx["tasks"])
	assert.Equal(t, "petId", idx["pets"])
	assert.Equal(t, "section_gid", idx["projects/*/sections"], "nested path uses namespace key")
	assert.NotContains(t, idx, "items") // no fetch/update/delete for items
}

// lastPathParam and related path utilities are tested in state/identity_test.go.
// They are exported from the state package and called by this package.

// --- resolveRef tests ---

func TestResolveRef(t *testing.T) {
	t.Parallel()

	t.Run("nil schema returns nil", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, resolveRef(nil))
	})

	t.Run("no ref returns same schema", func(t *testing.T) {
		t.Parallel()

		s := &jsonschema.Schema{Types: newTypes("object")}
		assert.Same(t, s, resolveRef(s))
	})

	t.Run("follows ref chain", func(t *testing.T) {
		t.Parallel()

		target := &jsonschema.Schema{Types: newTypes("object")}
		mid := &jsonschema.Schema{Ref: target}
		root := &jsonschema.Schema{Ref: mid}
		assert.Same(t, target, resolveRef(root))
	})
}

// --- hasType tests ---

func TestHasType(t *testing.T) {
	t.Parallel()

	t.Run("nil schema returns false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasType(nil, "object"))
	})

	t.Run("nil types returns false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, hasType(&jsonschema.Schema{}, "object"))
	})

	t.Run("matching type returns true", func(t *testing.T) {
		t.Parallel()

		s := &jsonschema.Schema{Types: newTypes("object")}
		assert.True(t, hasType(s, "object"))
	})

	t.Run("non-matching type returns false", func(t *testing.T) {
		t.Parallel()

		s := &jsonschema.Schema{Types: newTypes("string")}
		assert.False(t, hasType(s, "object"))
	})

	t.Run("type in allOf branch returns true", func(t *testing.T) {
		t.Parallel()

		s := &jsonschema.Schema{
			AllOf: []*jsonschema.Schema{
				{Types: newTypes("object")},
			},
		}
		assert.True(t, hasType(s, "object"))
	})

	t.Run("type in allOf branch via $ref returns true", func(t *testing.T) {
		t.Parallel()

		s := &jsonschema.Schema{
			AllOf: []*jsonschema.Schema{
				{Ref: &jsonschema.Schema{Types: newTypes("object")}},
			},
		}
		assert.True(t, hasType(s, "object"))
	})
}

// --- annotateStatefulMetadata integration test ---

func TestAnnotateStatefulMetadata(t *testing.T) {
	t.Parallel()

	// Build a minimal BehaviorMap with create + fetch + list for the same resource.
	// Simulates Asana's /projects pattern: wrapped single-resource, wrapped list, gid ID.
	bm := model.NewBehaviorMap()

	// Create: POST /projects — success schema wrapped in {data: {object}}
	bm.Put(model.BehaviorEntry{
		Method:      "POST",
		PathPattern: "/projects",
		Type:        model.BehaviorCreate,
		SuccessCode: 201,
		ResponseSchemas: map[int]*model.CompiledSchema{
			201: compiledSchema(&jsonschema.Schema{
				Types: newTypes("object"),
				Properties: map[string]*jsonschema.Schema{
					"data": {Types: newTypes("object")},
				},
			}),
		},
	})

	// Fetch: GET /projects/{project_gid} — same wrapper pattern
	bm.Put(model.BehaviorEntry{
		Method:      "GET",
		PathPattern: "/projects/{project_gid}",
		Type:        model.BehaviorFetch,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: compiledSchema(&jsonschema.Schema{
				Types: newTypes("object"),
				Properties: map[string]*jsonschema.Schema{
					"data": {Types: newTypes("object")},
				},
			}),
		},
	})

	// List: GET /projects — list response with {data: [array], has_more: boolean}
	bm.Put(model.BehaviorEntry{
		Method:      "GET",
		PathPattern: "/projects",
		Type:        model.BehaviorList,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: compiledSchema(&jsonschema.Schema{
				Types: newTypes("object"),
				Properties: map[string]*jsonschema.Schema{
					"data":     {Types: newTypes("array")},
					"has_more": {Types: newTypes("boolean")},
				},
			}),
		},
	})

	logger := slog.Default()
	annotateStatefulMetadata(bm, logger)

	// Verify create entry got WrapperKey and IDFieldHint.
	create, ok := bm.Get("POST", "/projects")
	require.True(t, ok)
	assert.Equal(t, "data", create.WrapperKey)
	assert.Equal(t, "gid", create.IDFieldHint)

	// Verify fetch entry got WrapperKey and IDFieldHint.
	fetch, ok := bm.Get("GET", "/projects/{project_gid}")
	require.True(t, ok)
	assert.Equal(t, "data", fetch.WrapperKey)
	assert.Equal(t, "gid", fetch.IDFieldHint)

	// Verify list entry got ListArrayKey.
	list, ok := bm.Get("GET", "/projects")
	require.True(t, ok)
	assert.Equal(t, "data", list.ListArrayKey)
	// List should also get IDFieldHint from the fetch entry.
	assert.Equal(t, "gid", list.IDFieldHint)
}

func TestAnnotateStatefulMetadata_FlatSpec(t *testing.T) {
	t.Parallel()

	// Petstore-style flat spec: no wrappers, "id" convention.
	bm := model.NewBehaviorMap()

	bm.Put(model.BehaviorEntry{
		Method:      "POST",
		PathPattern: "/pets",
		Type:        model.BehaviorCreate,
		SuccessCode: 201,
		ResponseSchemas: map[int]*model.CompiledSchema{
			201: compiledSchema(&jsonschema.Schema{
				Types: newTypes("object"),
				Properties: map[string]*jsonschema.Schema{
					"id":   {Types: newTypes("integer")},
					"name": {Types: newTypes("string")},
					"tag":  {Types: newTypes("string")},
				},
			}),
		},
	})

	bm.Put(model.BehaviorEntry{
		Method:      "GET",
		PathPattern: "/pets/{petId}",
		Type:        model.BehaviorFetch,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: compiledSchema(&jsonschema.Schema{
				Types: newTypes("object"),
				Properties: map[string]*jsonschema.Schema{
					"id":   {Types: newTypes("integer")},
					"name": {Types: newTypes("string")},
					"tag":  {Types: newTypes("string")},
				},
			}),
		},
	})

	bm.Put(model.BehaviorEntry{
		Method:      "GET",
		PathPattern: "/pets",
		Type:        model.BehaviorList,
		SuccessCode: 200,
		ResponseSchemas: map[int]*model.CompiledSchema{
			200: compiledSchema(&jsonschema.Schema{
				Types: newTypes("array"),
			}),
		},
	})

	logger := slog.Default()
	annotateStatefulMetadata(bm, logger)

	// Flat: no wrapper key.
	create, _ := bm.Get("POST", "/pets")
	assert.Empty(t, create.WrapperKey)
	// petId suffix-strips to "id" — but computeIDFieldHint correctly produces "id".
	assert.Equal(t, "id", create.IDFieldHint)

	// List: bare array, no ListArrayKey.
	list, _ := bm.Get("GET", "/pets")
	assert.Empty(t, list.ListArrayKey)
}
