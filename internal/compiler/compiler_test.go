package compiler_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mimikos-io/mimikos/internal/compiler"
	"github.com/mimikos-io/mimikos/internal/parser"
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
	p := parser.NewLibopenAPIParser(nil)

	spec, err := p.Parse(context.Background(), data)
	require.NoError(t, err)

	return data, spec
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

	sc, err := compiler.New(data, "3.0.0")
	require.NoError(t, err)
	require.NotNil(t, sc)
}

func TestNew_Petstore31(t *testing.T) {
	data := loadSpec(t, "petstore-3.1.yaml")

	sc, err := compiler.New(data, "3.1.0")
	require.NoError(t, err)
	require.NotNil(t, sc)
}

func TestNew_EmptyInput(t *testing.T) {
	_, err := compiler.New(nil, "3.0.0")
	require.ErrorIs(t, err, compiler.ErrEmptyInput)

	_, err = compiler.New([]byte{}, "3.0.0")
	require.ErrorIs(t, err, compiler.ErrEmptyInput)
}

func TestNew_InvalidYAML(t *testing.T) {
	_, err := compiler.New([]byte("not: valid: yaml: ["), "3.0.0")
	assert.ErrorIs(t, err, compiler.ErrInvalidSpec)
}

// --- Compile Component Schemas ---

func TestCompile_SimpleObjectSchema(t *testing.T) {
	data, spec := parseSpec(t, "petstore-3.0.yaml")

	sc, err := compiler.New(data, spec.Version)
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

	sc, err := compiler.New(data, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	sc, err := compiler.New(data, spec.Version)
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

	sc, err := compiler.New(data, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	sc, err := compiler.New(specBytes, spec.Version)
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

	sc, err := compiler.New(data, "3.0.0")
	require.NoError(t, err)

	// Pointer to non-existent schema.
	_, err = sc.Compile("#/components/schemas/NonExistent", "NonExistent", false)
	require.ErrorIs(t, err, compiler.ErrCompile)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	// Inline response schema.
	respRef := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, respRef)
	assert.Equal(t, "#/paths/~1pets~1{petId}/get/responses/200/content/application~1json/schema", respRef.Pointer)

	// Inline request body schema.
	reqRef := spec.Operations[1].RequestBody.Schema
	require.NotNil(t, reqRef)
	assert.Equal(t, "#/paths/~1pets~1{petId}/post/requestBody/content/application~1json/schema", reqRef.Pointer)
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

	p := parser.NewLibopenAPIParser(nil)
	spec, err := p.Parse(context.Background(), specBytes)
	require.NoError(t, err)

	ref := spec.Operations[0].Responses[200].Schema
	require.NotNil(t, ref)
	assert.Equal(t, "#/paths/~1items/get/responses/200/content/application~1vnd.api+json/schema", ref.Pointer)
}
