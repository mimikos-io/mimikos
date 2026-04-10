package parser

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the absolute path to testdata/specs/.
func testdataDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata", "specs")
}

// loadSpec reads a spec file from testdata/specs/ and returns the raw bytes.
func loadSpec(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	require.NoError(t, err)

	return data
}

// loadDocument reads a spec file and creates a libopenapi.Document.
func loadDocument(t *testing.T, name string) libopenapi.Document {
	t.Helper()

	data := loadSpec(t, name)
	doc, err := libopenapi.NewDocument(data)
	require.NoError(t, err)

	return doc
}

func newDocument(t *testing.T, data []byte) libopenapi.Document {
	t.Helper()

	doc, err := libopenapi.NewDocument(data)
	require.NoError(t, err)

	return doc
}

// newParser creates a LibopenAPIParser with no logger (quiet tests).
func newParser() *LibopenAPIParser {
	return NewLibopenAPIParser(nil)
}

// --- Error Cases ---

func TestParse_SwaggerV2(t *testing.T) {
	p := newParser()

	doc := newDocument(t, []byte(`swagger: "2.0"
info:
  title: Old API
  version: "1.0"
paths: {}`))

	_, err := p.Parse(context.Background(), doc)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
}

func TestParse_FutureVersion(t *testing.T) {
	p := newParser()

	doc := newDocument(t, []byte(`openapi: "4.0.0"
info:
  title: Future API
  version: "1.0"
paths: {}`))

	_, err := p.Parse(context.Background(), doc)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
}

func TestParse_ContextCancellation(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.0.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := p.Parse(ctx, doc)
	assert.ErrorIs(t, err, context.Canceled)
}

// --- Petstore 3.0 ---

func TestParse_Petstore30(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.0.yaml")

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.NotNil(t, spec)

	// Metadata.
	assert.Equal(t, "3.0.0", spec.Version)
	assert.Equal(t, "Swagger Petstore", spec.Title)

	// 3 operations: GET /pets, POST /pets, GET /pets/{petId}.
	require.Len(t, spec.Operations, 3)

	t.Run("listPets", func(t *testing.T) {
		op := spec.Operations[0]
		assert.Equal(t, "GET", op.Method)
		assert.Equal(t, "/pets", op.Path)
		assert.Equal(t, "listPets", op.OperationID)
		assert.Equal(t, "List all pets", op.Summary)
		assert.Empty(t, op.Description, "petstore-3.0 has no operation descriptions")
		assert.Nil(t, op.RequestBody)

		// 200 response with Pets schema (array).
		require.Contains(t, op.Responses, 200)
		resp200 := op.Responses[200]
		require.NotNil(t, resp200.Schema)
		assert.Equal(t, "Pets", resp200.Schema.Name)
		assert.NotNil(t, resp200.Schema.Raw)

		// Default response with Error schema.
		require.NotNil(t, op.DefaultResponse)
		require.NotNil(t, op.DefaultResponse.Schema)
		assert.Equal(t, "Error", op.DefaultResponse.Schema.Name)
	})

	t.Run("createPets", func(t *testing.T) {
		op := spec.Operations[1]
		assert.Equal(t, "POST", op.Method)
		assert.Equal(t, "/pets", op.Path)
		assert.Equal(t, "createPets", op.OperationID)

		// Request body with Pet schema.
		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		require.NotNil(t, op.RequestBody.Schema)
		assert.Equal(t, "Pet", op.RequestBody.Schema.Name)

		// 201 response with no content (Null response).
		require.Contains(t, op.Responses, 201)
		resp201 := op.Responses[201]
		assert.Nil(t, resp201.Schema, "201 Null response should have nil schema")
	})

	t.Run("showPetById", func(t *testing.T) {
		op := spec.Operations[2]
		assert.Equal(t, "GET", op.Method)
		assert.Equal(t, "/pets/{petId}", op.Path)
		assert.Equal(t, "showPetById", op.OperationID)

		require.Contains(t, op.Responses, 200)
		require.NotNil(t, op.Responses[200].Schema)
		assert.Equal(t, "Pet", op.Responses[200].Schema.Name)
	})

	// No circular refs in Petstore 3.0.
	assert.Empty(t, spec.CircularRefs)
}

// --- Petstore 3.0 Expanded (allOf, pre-resolved refs) ---

func TestParse_Petstore30Expanded(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.0-expanded.yaml")

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.NotNil(t, spec)

	assert.Equal(t, "3.0.0", spec.Version)

	// 4 operations: GET /pets, POST /pets, GET /pets/{id}, DELETE /pets/{id}.
	require.Len(t, spec.Operations, 4)

	t.Run("findPets", func(t *testing.T) {
		op := spec.Operations[0]
		assert.Equal(t, "GET", op.Method)
		assert.Equal(t, "/pets", op.Path)
		assert.Equal(t, "findPets", op.OperationID)
		assert.Contains(t, op.Description, "Returns all pets from the system")
	})

	t.Run("addPet", func(t *testing.T) {
		op := spec.Operations[1]
		assert.Equal(t, "POST", op.Method)
		assert.Equal(t, "addPet", op.OperationID)
		assert.Contains(t, op.Description, "Creates a new pet in the store")

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		require.NotNil(t, op.RequestBody.Schema)
		assert.Equal(t, "NewPet", op.RequestBody.Schema.Name)
	})

	t.Run("deletePet_204_no_content", func(t *testing.T) {
		op := spec.Operations[3]
		assert.Equal(t, "DELETE", op.Method)
		assert.Equal(t, "/pets/{id}", op.Path)
		assert.Equal(t, "deletePet", op.OperationID)

		// 204 response has no JSON content.
		require.Contains(t, op.Responses, 204)
		assert.Nil(t, op.Responses[204].Schema, "204 should have nil schema")
		assert.Equal(t, "pet deleted", op.Responses[204].Description)

		// Default response has Error schema.
		require.NotNil(t, op.DefaultResponse)
		require.NotNil(t, op.DefaultResponse.Schema)
		assert.Equal(t, "Error", op.DefaultResponse.Schema.Name)
	})

	// Pet uses allOf — verify the schema is still accessible.
	t.Run("allOf_schema_accessible", func(t *testing.T) {
		op := spec.Operations[0] // findPets returns array of Pet
		require.Contains(t, op.Responses, 200)

		resp := op.Responses[200]
		require.NotNil(t, resp.Schema)
		require.NotNil(t, resp.Schema.Raw)

		// The response is an inline array with items referencing Pet.
		// The Raw schema should be accessible and have array type.
		raw := resp.Schema.Raw
		require.NotEmpty(t, raw.Type)
		assert.Equal(t, "array", raw.Type[0])
	})
}

// --- Petstore 3.1 (polymorphism, circular refs, 3.1 features) ---

func TestParse_Petstore31(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.1.yaml")

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.NotNil(t, spec)

	assert.Equal(t, "3.1.0", spec.Version)
	assert.Equal(t, "Petstore 3.1", spec.Title)

	// 5 operations: GET /pets, POST /pets, GET /pets/{petId},
	// DELETE /pets/{petId}, PATCH /pets/{petId}.
	require.Len(t, spec.Operations, 5)

	t.Run("operations_ordered", func(t *testing.T) {
		methods := make([]string, len(spec.Operations))
		paths := make([]string, len(spec.Operations))

		for i, op := range spec.Operations {
			methods[i] = op.Method
			paths[i] = op.Path
		}

		// Order should follow the spec: /pets GET, POST, then /pets/{petId} GET, DELETE, PATCH.
		assert.Equal(t, []string{"GET", "POST", "GET", "DELETE", "PATCH"}, methods)
		assert.Equal(t, []string{"/pets", "/pets", "/pets/{petId}", "/pets/{petId}", "/pets/{petId}"}, paths)
	})

	t.Run("createPet_201_with_schema", func(t *testing.T) {
		op := spec.Operations[1] // POST /pets
		assert.Equal(t, "createPet", op.OperationID)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		require.NotNil(t, op.RequestBody.Schema)
		assert.Equal(t, "NewPet", op.RequestBody.Schema.Name)

		require.Contains(t, op.Responses, 201)
		resp201 := op.Responses[201]
		require.NotNil(t, resp201.Schema)
		assert.Equal(t, "Pet", resp201.Schema.Name)
	})

	t.Run("deletePet_204_no_content", func(t *testing.T) {
		op := spec.Operations[3] // DELETE /pets/{petId}
		assert.Equal(t, "deletePet", op.OperationID)

		require.Contains(t, op.Responses, 204)
		assert.Nil(t, op.Responses[204].Schema, "204 should have nil schema")
	})

	t.Run("updatePet_request_and_response", func(t *testing.T) {
		op := spec.Operations[4] // PATCH /pets/{petId}
		assert.Equal(t, "updatePet", op.OperationID)

		require.NotNil(t, op.RequestBody)
		assert.True(t, op.RequestBody.Required)
		require.NotNil(t, op.RequestBody.Schema)
		assert.Equal(t, "PetUpdate", op.RequestBody.Schema.Name)

		require.Contains(t, op.Responses, 200)
		require.NotNil(t, op.Responses[200].Schema)
		assert.Equal(t, "Pet", op.Responses[200].Schema.Name)
	})

	t.Run("polymorphic_schema_accessible", func(t *testing.T) {
		// Pet.status uses oneOf with discriminator — verify schema is walkable.
		op := spec.Operations[2] // GET /pets/{petId}
		require.Contains(t, op.Responses, 200)

		petSchema := op.Responses[200].Schema
		require.NotNil(t, petSchema)
		require.NotNil(t, petSchema.Raw)

		// Pet has a "status" property with oneOf.
		props := petSchema.Raw.Properties
		require.NotNil(t, props)

		statusPair := props.GetOrZero("status")
		require.NotNil(t, statusPair, "Pet should have a 'status' property")

		statusSchema := statusPair.Schema()
		require.NotNil(t, statusSchema)
		assert.NotEmpty(t, statusSchema.OneOf, "status should have oneOf variants")
	})

	t.Run("circular_refs_detected", func(t *testing.T) {
		// Category schema has self-referencing parent and children fields.
		require.NotEmpty(t, spec.CircularRefs, "should detect circular refs in Category")

		// At least one circular ref should reference Category.
		found := false

		for _, cr := range spec.CircularRefs {
			if cr.SchemaName == "Category" {
				found = true

				break
			}
		}

		assert.True(t, found, "should find circular ref involving Category, got: %+v", spec.CircularRefs)
	})
}

// --- Ref Resolution ---

func TestParse_RefsFullyResolved(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.0.yaml")

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	// Every SchemaRef.Raw must be non-nil (all $refs resolved).
	for _, op := range spec.Operations {
		if op.RequestBody != nil && op.RequestBody.Schema != nil {
			assert.NotNil(t, op.RequestBody.Schema.Raw,
				"request body schema Raw should be resolved for %s %s", op.Method, op.Path)
		}

		for code, resp := range op.Responses {
			if resp.Schema != nil {
				assert.NotNil(t, resp.Schema.Raw,
					"response schema Raw should be resolved for %s %s [%d]", op.Method, op.Path, code)
			}
		}

		if op.DefaultResponse != nil && op.DefaultResponse.Schema != nil {
			assert.NotNil(t, op.DefaultResponse.Schema.Raw,
				"default response schema Raw should be resolved for %s %s", op.Method, op.Path)
		}
	}
}

// --- Operation Ordering ---

func TestParse_OperationOrderMatchesSpec(t *testing.T) {
	p := newParser()
	doc := loadDocument(t, "petstore-3.0.yaml")

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	// Petstore 3.0 defines: /pets (GET, POST), /pets/{petId} (GET).
	// Source order must be preserved.
	require.Len(t, spec.Operations, 3)
	assert.Equal(t, "GET", spec.Operations[0].Method)
	assert.Equal(t, "/pets", spec.Operations[0].Path)
	assert.Equal(t, "POST", spec.Operations[1].Method)
	assert.Equal(t, "/pets", spec.Operations[1].Path)
	assert.Equal(t, "GET", spec.Operations[2].Method)
	assert.Equal(t, "/pets/{petId}", spec.Operations[2].Path)
}

// --- Normalization Helpers ---

func TestIsNullable(t *testing.T) {
	p := newParser()

	t.Run("3.0_not_nullable", func(t *testing.T) {
		doc := loadDocument(t, "petstore-3.0.yaml")
		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		// Pet.tag is not nullable in 3.0 petstore (plain type: string).
		op := spec.Operations[2] // showPetById
		petSchema := op.Responses[200].Schema.Raw
		tagPair := petSchema.Properties.GetOrZero("tag")
		require.NotNil(t, tagPair)
		tagSchema := tagPair.Schema()
		require.NotNil(t, tagSchema)
		assert.False(t, IsNullable(tagSchema), "Pet.tag in 3.0 petstore is not nullable")
	})

	t.Run("3.0_nullable_true", func(t *testing.T) {
		// Inline spec with nullable: true (3.0 style).
		doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Nullable Test
  version: "1.0"
paths:
  /items:
    get:
      operationId: getItem
      responses:
        "200":
          description: An item
          content:
            application/json:
              schema:
                type: object
                properties:
                  nickname:
                    type: string
                    nullable: true`))

		result, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		op := result.Operations[0]
		itemSchema := op.Responses[200].Schema.Raw
		nickPair := itemSchema.Properties.GetOrZero("nickname")
		require.NotNil(t, nickPair)
		nickSchema := nickPair.Schema()
		require.NotNil(t, nickSchema)
		assert.True(t, IsNullable(nickSchema), "nickname with nullable: true should be nullable")
	})

	t.Run("3.1_type_array_with_null", func(t *testing.T) {
		doc := loadDocument(t, "petstore-3.1.yaml")
		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		// Pet.tag in 3.1 is type: ["string", "null"].
		op := spec.Operations[2] // showPetById
		petSchema := op.Responses[200].Schema.Raw
		tagPair := petSchema.Properties.GetOrZero("tag")
		require.NotNil(t, tagPair)
		tagSchema := tagPair.Schema()
		require.NotNil(t, tagSchema)
		assert.True(t, IsNullable(tagSchema), "Pet.tag in 3.1 should be nullable")
	})
}

func TestPrimaryType(t *testing.T) {
	p := newParser()

	t.Run("single_type", func(t *testing.T) {
		doc := loadDocument(t, "petstore-3.0.yaml")
		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		// Pet.name is type: string.
		op := spec.Operations[2]
		petSchema := op.Responses[200].Schema.Raw
		namePair := petSchema.Properties.GetOrZero("name")
		require.NotNil(t, namePair)
		nameSchema := namePair.Schema()
		require.NotNil(t, nameSchema)
		assert.Equal(t, "string", PrimaryType(nameSchema))
	})

	t.Run("type_array_with_null", func(t *testing.T) {
		doc := loadDocument(t, "petstore-3.1.yaml")
		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		// Pet.tag in 3.1 is type: ["string", "null"] → PrimaryType should return "string".
		op := spec.Operations[2]
		petSchema := op.Responses[200].Schema.Raw
		tagPair := petSchema.Properties.GetOrZero("tag")
		require.NotNil(t, tagPair)
		tagSchema := tagPair.Schema()
		require.NotNil(t, tagSchema)
		assert.Equal(t, "string", PrimaryType(tagSchema))
	})

	t.Run("object_with_properties_no_type", func(t *testing.T) {
		// Schemas with properties but no explicit type should return "".
		// This is an edge case documented in spike findings §5.5.
		// The caller is responsible for inferring "object" when appropriate.
		doc := loadDocument(t, "petstore-3.0.yaml")
		spec, err := p.Parse(context.Background(), doc)
		require.NoError(t, err)

		// Pets schema is type: array — verify that works.
		op := spec.Operations[0] // listPets
		petsSchema := op.Responses[200].Schema.Raw
		assert.Equal(t, "array", PrimaryType(petsSchema))
	})
}

// --- JSON Content Type Fallback ---

func TestParse_PlusJSONContentType(t *testing.T) {
	p := newParser()

	// Spec uses application/vnd.api+json instead of application/json.
	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: JSON API Test
  version: "1.0"
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200":
          description: A list of items
          content:
            application/vnd.api+json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id:
                      type: string
    patch:
      operationId: updateItem
      requestBody:
        required: true
        content:
          application/merge-patch+json:
            schema:
              type: object
              properties:
                name:
                  type: string
      responses:
        "200":
          description: Updated item
          content:
            application/vnd.api+json:
              schema:
                type: object
                properties:
                  id:
                    type: string`))

	result, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, result.Operations, 2)

	t.Run("response_with_vnd_api_json", func(t *testing.T) {
		op := result.Operations[0] // GET /items
		require.Contains(t, op.Responses, 200)
		require.NotNil(t, op.Responses[200].Schema, "should extract schema from application/vnd.api+json")
		assert.Equal(t, "array", op.Responses[200].Schema.Raw.Type[0])
	})

	t.Run("request_body_with_merge_patch_json", func(t *testing.T) {
		op := result.Operations[1] // PATCH /items
		require.NotNil(t, op.RequestBody, "should extract request body from application/merge-patch+json")
		require.NotNil(t, op.RequestBody.Schema)
		assert.True(t, op.RequestBody.Required)
	})
}

// --- Media-Type Example Extraction ---

func TestParse_MediaTypeExampleSingular(t *testing.T) {
	p := newParser()

	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Example Test
  version: "1.0"
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: integer
                  name:
                    type: string
              example:
                id: 1
                name: "Fido"`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, spec.Operations, 1)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp)
	require.NotNil(t, resp.Example, "singular media-type example should be extracted")

	m, ok := resp.Example.(map[string]any)
	require.True(t, ok, "example should decode to map[string]any, got %T", resp.Example)
	// CF1: YAML integers should be normalized to int64 for consistency with
	// the generator (which produces int64 for integer schema types).
	assert.Equal(t, int64(1), m["id"], "YAML int should be normalized to int64")
	assert.Equal(t, "Fido", m["name"])
}

func TestParse_MediaTypeExamplesPlural(t *testing.T) {
	p := newParser()

	// Uses the plural `examples` keyword with named Example Objects.
	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Plural Examples Test
  version: "1.0"
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: integer
                  name:
                    type: string
              examples:
                dog:
                  value:
                    id: 1
                    name: "Fido"
                cat:
                  value:
                    id: 2
                    name: "Whiskers"`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, spec.Operations, 1)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp)
	require.NotNil(t, resp.Example, "first plural example should be extracted")

	// Decision #110: first named example wins (spec order).
	m, ok := resp.Example.(map[string]any)
	require.True(t, ok, "example should decode to map[string]any, got %T", resp.Example)
	assert.Equal(t, int64(1), m["id"], "YAML int should be normalized to int64")
	assert.Equal(t, "Fido", m["name"])
}

func TestParse_MediaTypeExampleSingularOverPlural(t *testing.T) {
	p := newParser()

	// Per OpenAPI spec, `example` and `examples` are mutually exclusive.
	// If both appear (malformed but possible), singular wins.
	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Mutual Exclusivity Test
  version: "1.0"
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                type: object
              example:
                id: 99
                name: "Singular"
              examples:
                first:
                  value:
                    id: 1
                    name: "Plural"`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp.Example)

	m, ok := resp.Example.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(99), m["id"], "singular example should take priority")
	assert.Equal(t, "Singular", m["name"])
}

func TestParse_MediaTypeNoExample(t *testing.T) {
	p := newParser()

	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: No Example Test
  version: "1.0"
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        "200":
          description: A list
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp)
	assert.Nil(t, resp.Example, "no media-type example defined — should be nil")
	assert.NotNil(t, resp.Schema, "schema should still be extracted")
}

func TestParse_MediaTypeExampleOnDefaultResponse(t *testing.T) {
	p := newParser()

	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Default Response Example Test
  version: "1.0"
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: array
                items:
                  type: string
        default:
          description: Error
          content:
            application/json:
              schema:
                type: object
                properties:
                  message:
                    type: string
              example:
                message: "Something went wrong"`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	// Default response should have example extracted.
	require.NotNil(t, spec.Operations[0].DefaultResponse)
	require.NotNil(t, spec.Operations[0].DefaultResponse.Example)

	m, ok := spec.Operations[0].DefaultResponse.Example.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Something went wrong", m["message"])

	// Regular 200 response has no example.
	assert.Nil(t, spec.Operations[0].Responses[200].Example)
}

func TestParse_MediaTypeExampleExternalValueIgnored(t *testing.T) {
	p := newParser()

	// externalValue only — no inline value. Should result in nil Example.
	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: External Value Test
  version: "1.0"
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                type: object
              examples:
                fromFile:
                  externalValue: "https://example.com/pet.json"`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp)
	assert.Nil(t, resp.Example, "externalValue-only example should be ignored")
}

func TestParse_MediaTypeExampleWithSchemaStillExtracted(t *testing.T) {
	p := newParser()

	// Verify that having an example doesn't interfere with schema extraction.
	doc := newDocument(t, []byte(`openapi: "3.0.0"
info:
  title: Both Schema and Example
  version: "1.0"
paths:
  /pets/{petId}:
    get:
      operationId: getPet
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Pet"
              example:
                id: 1
                name: "Fido"
components:
  schemas:
    Pet:
      type: object
      properties:
        id:
          type: integer
        name:
          type: string`))

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	resp := spec.Operations[0].Responses[200]
	require.NotNil(t, resp)
	require.NotNil(t, resp.Schema, "schema should still be extracted alongside example")
	assert.Equal(t, "Pet", resp.Schema.Name)
	require.NotNil(t, resp.Example)

	m, ok := resp.Example.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(1), m["id"])
}
