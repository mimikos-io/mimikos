package compiler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mimikos-io/mimikos/internal/parser"
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

// loadSpec reads a spec file from testdata/specs/.
func loadSpec(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	require.NoError(t, err)

	return data
}

// parseSpec parses a spec file and returns the raw bytes and parsed spec.
func parseSpec(t *testing.T, name string) ([]byte, *parser.ParsedSpec) {
	t.Helper()

	data := loadSpec(t, name)

	doc, err := libopenapi.NewDocument(data)
	require.NoError(t, err)

	p := parser.NewLibopenAPIParser(nil)

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	return data, spec
}

// parseInlineSpec creates a document and parses inline spec bytes.
func parseInlineSpec(t *testing.T, specBytes []byte) *parser.ParsedSpec {
	t.Helper()

	doc, err := libopenapi.NewDocument(specBytes)
	require.NoError(t, err)

	p := parser.NewLibopenAPIParser(nil)

	spec, err := p.Parse(context.Background(), doc)
	require.NoError(t, err)

	return spec
}

// unmarshalJSON is a helper to unmarshal a JSON string into any for validation.
func unmarshalJSON(t *testing.T, jsonStr string) any {
	t.Helper()

	var v any

	err := json.Unmarshal([]byte(jsonStr), &v)
	require.NoError(t, err)

	return v
}

// --- Constructor ---

func TestNew_Petstore30(t *testing.T) {
	data := loadSpec(t, "petstore-3.0.yaml")

	sc, err := New(data, "3.0.0")
	require.NoError(t, err)
	require.NotNil(t, sc)
}

func TestNew_Petstore31(t *testing.T) {
	data := loadSpec(t, "petstore-3.1.yaml")

	sc, err := New(data, "3.1.0")
	require.NoError(t, err)
	require.NotNil(t, sc)
}

func TestNew_EmptyInput(t *testing.T) {
	_, err := New(nil, "3.0.0")
	require.ErrorIs(t, err, ErrEmptyInput)

	_, err = New([]byte{}, "3.0.0")
	require.ErrorIs(t, err, ErrEmptyInput)
}

func TestNew_InvalidYAML(t *testing.T) {
	_, err := New([]byte("not: valid: yaml: ["), "3.0.0")
	assert.ErrorIs(t, err, ErrInvalidSpec)
}

// --- Compile Component Schemas ---

func TestCompile_SimpleObjectSchema(t *testing.T) {
	data, spec := parseSpec(t, "petstore-3.0.yaml")

	sc, err := New(data, spec.Version)
	require.NoError(t, err)

	// Find the Pet schema from showPetById response.
	op := spec.Operations[2] // GET /pets/{petId}
	petRef := op.Responses[200].Schema
	require.NotNil(t, petRef)
	assert.Equal(t, "#/components/schemas/Pet", petRef.Pointer)

	compiled, err := sc.Compile(petRef.Pointer, petRef.Name, petRef.IsCircular)
	require.NoError(t, err)
	require.NotNil(t, compiled)
	assert.Equal(t, "Pet", compiled.Name)
	assert.False(t, compiled.IsCircular)

	// Valid Pet document should pass.
	validPet := unmarshalJSON(t, `{"id": 1, "name": "Fido", "tag": "dog"}`)
	require.NoError(t, compiled.Validate(validPet))

	// Missing required "id" field should fail.
	missingID := unmarshalJSON(t, `{"name": "Fido"}`)
	require.Error(t, compiled.Validate(missingID))

	// Missing required "name" field should fail.
	missingName := unmarshalJSON(t, `{"id": 1}`)
	require.Error(t, compiled.Validate(missingName))
}

func TestCompile_ArraySchema(t *testing.T) {
	data, spec := parseSpec(t, "petstore-3.0.yaml")

	sc, err := New(data, spec.Version)
	require.NoError(t, err)

	// Pets schema (array of Pet) from listPets response.
	op := spec.Operations[0] // GET /pets
	petsRef := op.Responses[200].Schema
	require.NotNil(t, petsRef)
	assert.Equal(t, "#/components/schemas/Pets", petsRef.Pointer)

	compiled, err := sc.Compile(petsRef.Pointer, petsRef.Name, petsRef.IsCircular)
	require.NoError(t, err)

	// Valid array of pets should pass.
	validArray := unmarshalJSON(t, `[{"id": 1, "name": "Fido"}, {"id": 2, "name": "Rex"}]`)
	require.NoError(t, compiled.Validate(validArray))

	// Non-array should fail.
	notArray := unmarshalJSON(t, `{"id": 1, "name": "Fido"}`)
	require.Error(t, compiled.Validate(notArray))

	// Array with invalid item (missing required fields) should fail.
	invalidItem := unmarshalJSON(t, `[{"tag": "dog"}]`)
	require.Error(t, compiled.Validate(invalidItem))
}

func TestCompile_SchemaWithConstraints(t *testing.T) {
	// Spec with required, minLength, enum, minimum/maximum constraints.
	specBytes := []byte(`openapi: "3.1.0"
info:
  title: Constraints Test
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
                $ref: "#/components/schemas/Item"
components:
  schemas:
    Item:
      type: object
      required:
        - name
        - status
        - count
      properties:
        name:
          type: string
          minLength: 1
        status:
          type: string
          enum:
            - active
            - archived
        count:
          type: integer
          minimum: 0
          maximum: 100`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	itemRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(itemRef.Pointer, itemRef.Name, itemRef.IsCircular)
	require.NoError(t, err)

	// Valid item.
	valid := unmarshalJSON(t, `{"name": "test", "status": "active", "count": 50}`)
	require.NoError(t, compiled.Validate(valid))

	// Empty name (minLength: 1 violated).
	emptyName := unmarshalJSON(t, `{"name": "", "status": "active", "count": 50}`)
	require.Error(t, compiled.Validate(emptyName))

	// Invalid enum value.
	badEnum := unmarshalJSON(t, `{"name": "test", "status": "deleted", "count": 50}`)
	require.Error(t, compiled.Validate(badEnum))

	// Count out of range (minimum: 0).
	negativeCount := unmarshalJSON(t, `{"name": "test", "status": "active", "count": -1}`)
	require.Error(t, compiled.Validate(negativeCount))

	// Count out of range (maximum: 100).
	overMax := unmarshalJSON(t, `{"name": "test", "status": "active", "count": 101}`)
	require.Error(t, compiled.Validate(overMax))
}

// --- OneOf Schema ---

func TestCompile_OneOfSchema(t *testing.T) {
	data, spec := parseSpec(t, "petstore-3.1.yaml")

	sc, err := New(data, spec.Version)
	require.NoError(t, err)

	// Pet schema has oneOf for status field.
	petRef := spec.Operations[2].Responses[200].Schema // GET /pets/{petId} → 200
	compiled, err := sc.Compile(petRef.Pointer, petRef.Name, petRef.IsCircular)
	require.NoError(t, err)

	// Valid pet with ActiveStatus variant.
	activePet := unmarshalJSON(t, `{
		"id": 1,
		"name": "Fido",
		"status": {"type": "active", "since": "2024-01-01"}
	}`)
	require.NoError(t, compiled.Validate(activePet))

	// Valid pet with ArchivedStatus variant.
	archivedPet := unmarshalJSON(t, `{
		"id": 2,
		"name": "Rex",
		"status": {"type": "archived", "reason": "relocated"}
	}`)
	require.NoError(t, compiled.Validate(archivedPet))

	// Pet with null tag (3.1 nullable via type array).
	nullTagPet := unmarshalJSON(t, `{
		"id": 3,
		"name": "Buddy",
		"tag": null
	}`)
	require.NoError(t, compiled.Validate(nullTagPet))
}

// --- Circular Schema ---

func TestCompile_CircularSchema(t *testing.T) {
	data, spec := parseSpec(t, "petstore-3.1.yaml")

	// Verify Category is detected as circular by the parser.
	require.NotEmpty(t, spec.CircularRefs)

	sc, err := New(data, spec.Version)
	require.NoError(t, err)

	// Compile the circular Category schema directly by pointer.
	compiled, err := sc.Compile("#/components/schemas/Category", "Category", true)
	require.NoError(t, err, "circular schema should compile without infinite loop")
	require.NotNil(t, compiled)
	assert.True(t, compiled.IsCircular)

	// Valid Category with nested parent (parent is optional, omitted terminates recursion).
	validCategory := unmarshalJSON(t, `{
		"name": "Animals",
		"parent": {
			"name": "Living Things"
		},
		"children": [
			{"name": "Dogs"},
			{"name": "Cats"}
		]
	}`)
	require.NoError(t, compiled.Validate(validCategory))

	// Simple category without nesting.
	simpleCategory := unmarshalJSON(t, `{"name": "Pets"}`)
	require.NoError(t, compiled.Validate(simpleCategory))
}

// --- OpenAPI 3.0 Normalization ---

func TestCompile_30NullableNormalization(t *testing.T) {
	specBytes := []byte(`openapi: "3.0.0"
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
                $ref: "#/components/schemas/Item"
components:
  schemas:
    Item:
      type: object
      required:
        - name
      properties:
        name:
          type: string
        nickname:
          type: string
          nullable: true`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	itemRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(itemRef.Pointer, itemRef.Name, itemRef.IsCircular)
	require.NoError(t, err)

	// nickname: null should be valid (nullable: true normalized to type array with null).
	withNull := unmarshalJSON(t, `{"name": "test", "nickname": null}`)
	require.NoError(t, compiled.Validate(withNull))

	// nickname: "foo" should also be valid.
	withString := unmarshalJSON(t, `{"name": "test", "nickname": "foo"}`)
	require.NoError(t, compiled.Validate(withString))

	// nickname: 123 should fail (not string or null).
	withNumber := unmarshalJSON(t, `{"name": "test", "nickname": 123}`)
	require.Error(t, compiled.Validate(withNumber))
}

func TestCompile_30NullableWithRef(t *testing.T) {
	// OpenAPI 3.0 pattern: nullable: true next to $ref (e.g., Stripe expandable fields).
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: Nullable Ref Test
  version: "1.0"
paths:
  /orders:
    get:
      operationId: getOrder
      responses:
        "200":
          description: An order
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Order"
components:
  schemas:
    Order:
      type: object
      required:
        - id
      properties:
        id:
          type: integer
        customer:
          nullable: true
          $ref: "#/components/schemas/Customer"
    Customer:
      type: object
      required:
        - name
      properties:
        name:
          type: string`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	orderRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(orderRef.Pointer, orderRef.Name, orderRef.IsCircular)
	require.NoError(t, err)

	// customer: null should be valid (nullable + $ref normalized to anyOf).
	withNull := unmarshalJSON(t, `{"id": 1, "customer": null}`)
	require.NoError(t, compiled.Validate(withNull))

	// customer: {name: "Alice"} should be valid.
	withCustomer := unmarshalJSON(t, `{"id": 1, "customer": {"name": "Alice"}}`)
	require.NoError(t, compiled.Validate(withCustomer))

	// customer: 123 should fail (not Customer or null).
	withNumber := unmarshalJSON(t, `{"id": 1, "customer": 123}`)
	require.Error(t, compiled.Validate(withNumber))

	// customer: {} should fail (missing required "name" field).
	withEmpty := unmarshalJSON(t, `{"id": 1, "customer": {}}`)
	require.Error(t, compiled.Validate(withEmpty))
}

func TestCompile_30ExclusiveMinimumNormalization(t *testing.T) {
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: ExclusiveMin Test
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
                $ref: "#/components/schemas/Item"
components:
  schemas:
    Item:
      type: object
      properties:
        score:
          type: number
          minimum: 0
          exclusiveMinimum: true
        rating:
          type: number
          maximum: 10
          exclusiveMaximum: true`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	itemRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(itemRef.Pointer, itemRef.Name, itemRef.IsCircular)
	require.NoError(t, err)

	// score: 0 should fail (exclusive minimum = 0, so 0 itself is excluded).
	zeroScore := unmarshalJSON(t, `{"score": 0}`)
	require.Error(t, compiled.Validate(zeroScore))

	// score: 0.01 should pass.
	positiveScore := unmarshalJSON(t, `{"score": 0.01}`)
	require.NoError(t, compiled.Validate(positiveScore))

	// rating: 10 should fail (exclusive maximum = 10).
	maxRating := unmarshalJSON(t, `{"rating": 10}`)
	require.Error(t, compiled.Validate(maxRating))

	// rating: 9.99 should pass.
	validRating := unmarshalJSON(t, `{"rating": 9.99}`)
	require.NoError(t, compiled.Validate(validRating))
}

func TestCompile_30ExclusiveFalseNormalization(t *testing.T) {
	// exclusiveMinimum: false / exclusiveMaximum: false should be removed,
	// leaving regular minimum/maximum in effect.
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: ExclusiveFalse Test
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
                $ref: "#/components/schemas/Item"
components:
  schemas:
    Item:
      type: object
      properties:
        score:
          type: number
          minimum: 0
          exclusiveMinimum: false
        rating:
          type: number
          maximum: 10
          exclusiveMaximum: false`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	itemRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(itemRef.Pointer, itemRef.Name, itemRef.IsCircular)
	require.NoError(t, err)

	// score: 0 should pass (minimum is inclusive when exclusiveMinimum: false).
	zeroScore := unmarshalJSON(t, `{"score": 0}`)
	require.NoError(t, compiled.Validate(zeroScore))

	// score: -1 should fail (below minimum).
	negativeScore := unmarshalJSON(t, `{"score": -1}`)
	require.Error(t, compiled.Validate(negativeScore))

	// rating: 10 should pass (maximum is inclusive when exclusiveMaximum: false).
	maxRating := unmarshalJSON(t, `{"rating": 10}`)
	require.NoError(t, compiled.Validate(maxRating))

	// rating: 11 should fail (above maximum).
	overMax := unmarshalJSON(t, `{"rating": 11}`)
	require.Error(t, compiled.Validate(overMax))
}

// --- Inline Schema ---

func TestCompile_InlineSchema(t *testing.T) {
	specBytes := []byte(`openapi: "3.1.0"
info:
  title: Inline Test
  version: "1.0"
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200":
          description: Items
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  required:
                    - id
                  properties:
                    id:
                      type: integer`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	// Inline schema should have a full JSON pointer path (not a $ref).
	inlineRef := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, inlineRef)
	assert.Equal(t, "#/paths/~1items/get/responses/200/content/application~1json/schema", inlineRef.Pointer)

	compiled, err := sc.Compile(inlineRef.Pointer, inlineRef.Name, inlineRef.IsCircular)
	require.NoError(t, err)

	// Valid array.
	valid := unmarshalJSON(t, `[{"id": 1}, {"id": 2}]`)
	require.NoError(t, compiled.Validate(valid))

	// Non-array fails.
	notArray := unmarshalJSON(t, `{"id": 1}`)
	require.Error(t, compiled.Validate(notArray))
}

// --- Error Cases ---

func TestCompile_InvalidPointer(t *testing.T) {
	data := loadSpec(t, "petstore-3.0.yaml")

	sc, err := New(data, "3.0.0")
	require.NoError(t, err)

	// Pointer to non-existent schema.
	_, err = sc.Compile("#/components/schemas/NonExistent", "NonExistent", false)
	require.ErrorIs(t, err, ErrCompile)
}

// --- Pointer Values on Parsed Specs ---

func TestPointer_ComponentRef(t *testing.T) {
	_, spec := parseSpec(t, "petstore-3.0.yaml")

	// showPetById → 200 → Pet (component $ref).
	petRef := spec.Operations[2].Responses[200].Schema
	require.NotNil(t, petRef)
	assert.Equal(t, "#/components/schemas/Pet", petRef.Pointer)

	// listPets → 200 → Pets (component $ref).
	petsRef := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, petsRef)
	assert.Equal(t, "#/components/schemas/Pets", petsRef.Pointer)

	// listPets → default → Error (component $ref).
	errorRef := spec.Operations[0].DefaultResponse.Schema
	require.NotNil(t, errorRef)
	assert.Equal(t, "#/components/schemas/Error", errorRef.Pointer)

	// createPets → request body → Pet (component $ref).
	reqRef := spec.Operations[1].RequestBody.Schema
	require.NotNil(t, reqRef)
	assert.Equal(t, "#/components/schemas/Pet", reqRef.Pointer)
}

func TestPointer_InlineSchema(t *testing.T) {
	specBytes := []byte(`openapi: "3.1.0"
info:
  title: Inline Pointer Test
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
    post:
      operationId: createPet
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
      responses:
        "201":
          description: Created`)

	spec := parseInlineSpec(t, specBytes)

	// Inline response schema.
	respRef := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, respRef)
	assert.Equal(t, "#/paths/~1pets~1{petId}/get/responses/200/content/application~1json/schema", respRef.Pointer)

	// Inline request body schema.
	reqRef := spec.Operations[1].RequestBody.Schema
	require.NotNil(t, reqRef)
	assert.Equal(t, "#/paths/~1pets~1{petId}/post/requestBody/content/application~1json/schema", reqRef.Pointer)
}

// --- Example Normalization ---

func TestNormalizeExamples_StringExample(t *testing.T) {
	doc := map[string]any{
		"type":    "string",
		"example": "Fido",
	}

	normalizeExamples(doc)

	assert.Equal(t, []any{"Fido"}, doc["examples"])
	assert.NotContains(t, doc, "example", "example key should be removed")
}

func TestNormalizeExamples_IntegerExample(t *testing.T) {
	doc := map[string]any{
		"type":    "integer",
		"example": 42,
	}

	normalizeExamples(doc)

	assert.Equal(t, []any{42}, doc["examples"])
	assert.NotContains(t, doc, "example")
}

func TestNormalizeExamples_BothExampleAndExamples(t *testing.T) {
	// When both exist, examples (array) is preserved and example is removed.
	doc := map[string]any{
		"type":     "string",
		"example":  "ignored",
		"examples": []any{"kept"},
	}

	normalizeExamples(doc)

	assert.Equal(t, []any{"kept"}, doc["examples"], "existing examples should be preserved")
	assert.NotContains(t, doc, "example", "example key should be removed")
}

func TestNormalizeExamples_NoExample(t *testing.T) {
	doc := map[string]any{
		"type": "string",
	}

	normalizeExamples(doc)

	assert.NotContains(t, doc, "examples", "should not add examples when none present")
}

func TestNormalizeExamples_NestedSchemas(t *testing.T) {
	doc := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":    "string",
				"example": "Fido",
			},
			"age": map[string]any{
				"type":    "integer",
				"example": 3,
			},
		},
	}

	normalizeExamples(doc)

	props, ok := doc["properties"].(map[string]any)
	require.True(t, ok, "properties should be map[string]any")

	nameSchema, ok := props["name"].(map[string]any)
	require.True(t, ok, "name property should be map[string]any")

	ageSchema, ok := props["age"].(map[string]any)
	require.True(t, ok, "age property should be map[string]any")

	assert.Equal(t, []any{"Fido"}, nameSchema["examples"])
	assert.NotContains(t, nameSchema, "example")
	assert.Equal(t, []any{3}, ageSchema["examples"])
	assert.NotContains(t, ageSchema, "example")
}

func TestNormalizeExamples_ArrayItems(t *testing.T) {
	doc := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":    "string",
			"example": "hello",
		},
	}

	normalizeExamples(doc)

	items, ok := doc["items"].(map[string]any)
	require.True(t, ok, "items should be map[string]any")
	assert.Equal(t, []any{"hello"}, items["examples"])
	assert.NotContains(t, items, "example")
}

func TestNormalizeExamples_PropertyNamedExample_Preserved(t *testing.T) {
	// A property *named* "example" in a properties map must not be destroyed.
	// The properties map itself has no schema keywords — looksLikeSchema should
	// return false, skipping normalization on the container.
	doc := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type": "string",
			},
			"example": map[string]any{
				"type":        "string",
				"description": "Usage example for this tutorial",
			},
		},
	}

	normalizeExamples(doc)

	props, ok := doc["properties"].(map[string]any)
	require.True(t, ok, "properties should be map[string]any")
	assert.Contains(t, props, "example", "property named 'example' must survive normalization")
	assert.NotContains(t, props, "examples", "should not create bogus 'examples' key in properties map")

	// The property schema itself should be untouched (no example keyword on it).
	exampleProp, ok := props["example"].(map[string]any)
	require.True(t, ok, "example property should be map[string]any")
	assert.Equal(t, "string", exampleProp["type"])
}

func TestNormalizeExamples_SchemaNamedExample_Preserved(t *testing.T) {
	// A schema named "example" in components/schemas must not be destroyed.
	doc := map[string]any{
		"components": map[string]any{
			"schemas": map[string]any{
				"example": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}

	normalizeExamples(doc)

	components, ok := doc["components"].(map[string]any)
	require.True(t, ok, "components should be map[string]any")

	schemas, ok := components["schemas"].(map[string]any)
	require.True(t, ok, "schemas should be map[string]any")

	assert.Contains(t, schemas, "example", "schema named 'example' must survive normalization")
	assert.NotContains(t, schemas, "examples")
}

func TestNormalizeExamples_NonSchemaMap_Unchanged(t *testing.T) {
	// A map with "example" key but no schema keywords should not be modified.
	doc := map[string]any{
		"description": "some context",
		"example":     "some value",
	}

	normalizeExamples(doc)

	assert.Contains(t, doc, "example", "non-schema map should not be modified")
	assert.NotContains(t, doc, "examples")
}

func TestLooksLikeSchema(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]any
		want bool
	}{
		{"type keyword", map[string]any{"type": "string"}, true},
		{"$ref keyword", map[string]any{"$ref": "#/components/schemas/Pet"}, true},
		{"allOf keyword", map[string]any{"allOf": []any{}}, true},
		{"oneOf keyword", map[string]any{"oneOf": []any{}}, true},
		{"anyOf keyword", map[string]any{"anyOf": []any{}}, true},
		{"enum keyword", map[string]any{"enum": []any{"a", "b"}}, true},
		{"const keyword", map[string]any{"const": "fixed"}, true},
		{"format keyword", map[string]any{"format": "email"}, true},
		{"properties keyword", map[string]any{"properties": map[string]any{}}, true},
		{"items keyword", map[string]any{"items": map[string]any{}}, true},
		{"minimum keyword", map[string]any{"minimum": 0}, true},
		{"maximum keyword", map[string]any{"maximum": 100}, true},
		{"minLength keyword", map[string]any{"minLength": 1}, true},
		{"maxLength keyword", map[string]any{"maxLength": 255}, true},
		{"pattern keyword", map[string]any{"pattern": "^[a-z]+$"}, true},
		{"no schema keywords", map[string]any{"description": "not a schema"}, false},
		{"empty map", map[string]any{}, false},
		{"properties container", map[string]any{"name": map[string]any{"type": "string"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeSchema(tt.obj))
		})
	}
}

// TestNormalizeExamples_Integration verifies that example normalization produces
// schemas with Examples populated after compilation.
func TestNormalizeExamples_Integration(t *testing.T) {
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: Example Test
  version: "1.0"
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        "200":
          description: A list of pets
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Pet"
components:
  schemas:
    Pet:
      type: object
      required:
        - name
      properties:
        name:
          type: string
          example: "Fido"
        age:
          type: integer
          example: 3
        vaccinated:
          type: boolean
          example: true
        weight:
          type: number
          example: 4.5
        tag:
          type: string`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	petRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(petRef.Pointer, petRef.Name, petRef.IsCircular)
	require.NoError(t, err)

	// Verify that the compiled schema's properties have Examples populated.
	nameSchema := compiled.Schema.Properties["name"]
	require.NotNil(t, nameSchema, "name property schema should exist")
	assert.Equal(t, []any{"Fido"}, nameSchema.Examples, "name should have example 'Fido'")

	ageSchema := compiled.Schema.Properties["age"]
	require.NotNil(t, ageSchema, "age property schema should exist")
	assert.Equal(t, []any{3}, ageSchema.Examples, "age should have example 3")

	vaccinatedSchema := compiled.Schema.Properties["vaccinated"]
	require.NotNil(t, vaccinatedSchema, "vaccinated property schema should exist")
	assert.Equal(t, []any{true}, vaccinatedSchema.Examples, "vaccinated should have example true")

	weightSchema := compiled.Schema.Properties["weight"]
	require.NotNil(t, weightSchema, "weight property schema should exist")
	assert.Equal(t, []any{4.5}, weightSchema.Examples, "weight should have example 4.5")

	// tag has no example — should have nil Examples.
	tagSchema := compiled.Schema.Properties["tag"]
	require.NotNil(t, tagSchema, "tag property schema should exist")
	assert.Nil(t, tagSchema.Examples, "tag should have no examples")
}

func TestPointer_PlusJSONContentType(t *testing.T) {
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: Plus JSON Pointer Test
  version: "1.0"
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200":
          description: Items
          content:
            application/vnd.api+json:
              schema:
                type: array
                items:
                  type: string`)

	spec := parseInlineSpec(t, specBytes)

	ref := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, ref)
	assert.Equal(t, "#/paths/~1items/get/responses/200/content/application~1vnd.api+json/schema", ref.Pointer)
}

// --- Response-Level $ref Compilation ---

func TestCompile_ResponseRefSchema(t *testing.T) {
	// Spec uses $ref at the response level. The parser produces component-based
	// pointers. The compiler must successfully compile those pointers.
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: Response Ref Compile Test
  version: "1.0"
paths:
  /items/{id}:
    get:
      operationId: getItem
      responses:
        "200":
          description: An item
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Item"
        "403":
          $ref: "#/components/responses/Forbidden"
        "429":
          $ref: "#/components/responses/TooManyRequests"
components:
  schemas:
    Item:
      type: object
      properties:
        id:
          type: integer
    ErrorObject:
      type: object
      properties:
        message:
          type: string
  responses:
    Forbidden:
      description: Forbidden
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                $ref: "#/components/schemas/ErrorObject"
    TooManyRequests:
      description: Too many requests
      content:
        application/json:
          schema:
            type: object
            properties:
              error:
                $ref: "#/components/schemas/ErrorObject"`)

	spec := parseInlineSpec(t, specBytes)
	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	op := spec.Operations[0]

	t.Run("200_schema_ref_compiles", func(t *testing.T) {
		ref := op.Responses[200].Schema
		require.NotNil(t, ref)

		compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
		require.NoError(t, err)
		require.NotNil(t, compiled)

		valid := unmarshalJSON(t, `{"id": 42}`)
		require.NoError(t, compiled.Validate(valid))
	})

	t.Run("403_response_ref_compiles", func(t *testing.T) {
		ref := op.Responses[403].Schema
		require.NotNil(t, ref)
		assert.Contains(t, ref.Pointer, "#/components/responses/Forbidden",
			"pointer should be component-based, not path-based")

		compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
		require.NoError(t, err, "compilation must succeed for response-level $ref pointer")
		require.NotNil(t, compiled)

		valid := unmarshalJSON(t, `{"error": {"message": "forbidden"}}`)
		require.NoError(t, compiled.Validate(valid))
	})

	t.Run("429_response_ref_compiles", func(t *testing.T) {
		ref := op.Responses[429].Schema
		require.NotNil(t, ref)

		compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
		require.NoError(t, err, "compilation must succeed for response-level $ref pointer")
		require.NotNil(t, compiled)
	})
}

func TestCompile_RequestBodyRefSchema(t *testing.T) {
	// Spec uses $ref at the request body level.
	specBytes := []byte(`openapi: "3.0.0"
info:
  title: RequestBody Ref Compile Test
  version: "1.0"
paths:
  /items:
    post:
      operationId: createItem
      requestBody:
        $ref: "#/components/requestBodies/ItemCreate"
      responses:
        "201":
          description: Created
components:
  requestBodies:
    ItemCreate:
      required: true
      content:
        application/json:
          schema:
            type: object
            properties:
              name:
                type: string
            required:
              - name`)

	spec := parseInlineSpec(t, specBytes)
	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	op := spec.Operations[0]
	require.NotNil(t, op.RequestBody)
	require.NotNil(t, op.RequestBody.Schema)

	ref := op.RequestBody.Schema
	assert.Contains(t, ref.Pointer, "#/components/requestBodies/ItemCreate",
		"pointer should be component-based, not path-based")

	compiled, err := sc.Compile(ref.Pointer, ref.Name, ref.IsCircular)
	require.NoError(t, err, "compilation must succeed for request body $ref pointer")
	require.NotNil(t, compiled)

	// Valid request body.
	valid := unmarshalJSON(t, `{"name": "Widget"}`)
	require.NoError(t, compiled.Validate(valid))

	// Missing required field.
	invalid := unmarshalJSON(t, `{}`)
	require.Error(t, compiled.Validate(invalid))
}

// --- Format Assertion ---

func TestCompile_FormatAssertion(t *testing.T) {
	// This test verifies that the compiler enables format assertion so that
	// schema.Format is populated on compiled schemas. Without AssertFormat(),
	// schema.Format is always nil in Draft 2020-12 (the default).
	specBytes := []byte(`openapi: "3.1.0"
info:
  title: Format Assertion Test
  version: "1.0"
paths:
  /events:
    get:
      operationId: listEvents
      responses:
        "200":
          description: Events
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Event"
components:
  schemas:
    Event:
      type: object
      properties:
        name:
          type: string
        started_at:
          type: string
          format: date-time
        date:
          type: string
          format: date
        contact:
          type: string
          format: email
        event_id:
          type: string
          format: uuid
        website:
          type: string
          format: uri
        host:
          type: string
          format: hostname
        server_ip:
          type: string
          format: ipv4
        server_ipv6:
          type: string
          format: ipv6`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	eventRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(eventRef.Pointer, eventRef.Name, eventRef.IsCircular)
	require.NoError(t, err)

	// Every format-annotated property must have a non-nil Format field.
	formatFields := map[string]string{
		"started_at":  "date-time",
		"date":        "date",
		"contact":     "email",
		"event_id":    "uuid",
		"website":     "uri",
		"host":        "hostname",
		"server_ip":   "ipv4",
		"server_ipv6": "ipv6",
	}

	for field, expectedFormat := range formatFields {
		prop := compiled.Schema.Properties[field]
		require.NotNil(t, prop, "property %q should exist", field)
		require.NotNil(t, prop.Format, "property %q should have Format populated (was nil before AssertFormat fix)", field)
		assert.Equal(t, expectedFormat, prop.Format.Name, "property %q format name", field)
	}

	// "name" has no format — should remain nil.
	nameProp := compiled.Schema.Properties["name"]
	require.NotNil(t, nameProp)
	assert.Nil(t, nameProp.Format, "property without format annotation should have nil Format")
}

func TestCompile_FormatValidation(t *testing.T) {
	// With AssertFormat(), the validator also enforces format constraints.
	// Verify that invalid format values are rejected.
	specBytes := []byte(`openapi: "3.1.0"
info:
  title: Format Validation Test
  version: "1.0"
paths:
  /events:
    get:
      operationId: listEvents
      responses:
        "200":
          description: Events
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Event"
components:
  schemas:
    Event:
      type: object
      properties:
        started_at:
          type: string
          format: date-time
        contact:
          type: string
          format: email`)

	spec := parseInlineSpec(t, specBytes)

	sc, err := New(specBytes, spec.Version)
	require.NoError(t, err)

	eventRef := spec.Operations[0].Responses[200].Schema
	compiled, err := sc.Compile(eventRef.Pointer, eventRef.Name, eventRef.IsCircular)
	require.NoError(t, err)

	// Valid format values should pass.
	valid := unmarshalJSON(t, `{"started_at": "2024-08-15T14:30:00Z", "contact": "user@example.com"}`)
	require.NoError(t, compiled.Validate(valid))

	// Invalid date-time format should fail.
	badDateTime := unmarshalJSON(t, `{"started_at": "not-a-date"}`)
	require.Error(t, compiled.Validate(badDateTime), "invalid date-time should be rejected with AssertFormat")

	// Invalid email format should fail.
	badEmail := unmarshalJSON(t, `{"contact": "not-an-email"}`)
	require.Error(t, compiled.Validate(badEmail), "invalid email should be rejected with AssertFormat")
}
