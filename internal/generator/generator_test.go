package generator

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Object generation tests (8.4.3) ---

func TestGenerateSimpleObject(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"age":  {Types: newTypes("integer")},
	}, "name", "age")

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", val)
	assert.Contains(t, m, "name")
	assert.Contains(t, m, "age")
	assert.IsType(t, "", m["name"])
	assert.IsType(t, int64(0), m["age"])
}

func TestGenerateObjectOptionalFieldsIncluded(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"bio":  {Types: newTypes("string")},
	}, "name") // only "name" is required

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, m, "name", "required field must be present")
	assert.Contains(t, m, "bio", "optional field should also be generated")
}

func TestGenerateNestedObject(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"address": objectSchema(map[string]*jsonschema.Schema{
			"city": {Types: newTypes("string")},
			"zip":  {Types: newTypes("string")},
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")

	addr, ok := m["address"].(map[string]any)
	require.True(t, ok, "nested object should be map[string]any")
	assert.Contains(t, addr, "city")
	assert.Contains(t, addr, "zip")
}

func TestGenerateObjectPerFieldSubSeeding(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"name":  {Types: newTypes("string")},
		"email": {Types: newTypes("string")},
	})

	val1, err := g.Generate(schema, 42)
	require.NoError(t, err)

	val2, err := g.Generate(schema, 42)
	require.NoError(t, err)

	// Same seed → identical output.
	assert.Equal(t, val1, val2, "same seed must produce identical output")
}

func TestGenerateObjectFieldIndependence(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Schema with two fields.
	schema1 := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"age":  {Types: newTypes("integer")},
	})

	// Schema with an extra field.
	schema2 := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"age":  {Types: newTypes("integer")},
		"bio":  {Types: newTypes("string")},
	})

	val1, err := g.Generate(schema1, 42)
	require.NoError(t, err)

	val2, err := g.Generate(schema2, 42)
	require.NoError(t, err)

	m1, ok := val1.(map[string]any)
	require.True(t, ok)

	m2, ok := val2.(map[string]any)
	require.True(t, ok)

	// Adding "bio" should not change "name" or "age" values.
	assert.Equal(t, m1["name"], m2["name"], "name should be field-independent")
	assert.Equal(t, m1["age"], m2["age"], "age should be field-independent")
}

func TestGenerateEmptyObject(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, m)
}

func TestGenerateMapType(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:                newTypes("object"),
		AdditionalProperties: &jsonschema.Schema{Types: newTypes("string")},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, m, "map-type schema should generate entries")

	for k, v := range m {
		assert.IsType(t, "", k, "keys should be strings")
		assert.IsType(t, "", v, "values should be strings")
	}
}

func TestGenerateObjectDeterminism(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"id":   {Types: newTypes("integer")},
		"name": {Types: newTypes("string")},
		"tags": {
			Types:     newTypes("array"),
			Items2020: &jsonschema.Schema{Types: newTypes("string")},
		},
	})

	results := make([]any, 5)
	for i := range results {
		val, err := g.Generate(schema, 99)
		require.NoError(t, err)

		results[i] = val
	}

	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i], "call %d should match call 0", i)
	}
}

// --- Array generation tests (8.4.3) ---

func TestGenerateSimpleArray(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any, got %T", val)
	assert.Len(t, arr, 3, "default array length should be 3")

	for i, item := range arr {
		assert.IsType(t, "", item, "item %d should be string", i)
	}
}

func TestGenerateArrayOfObjects(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		Items2020: objectSchema(map[string]*jsonschema.Schema{
			"id":   {Types: newTypes("integer")},
			"name": {Types: newTypes("string")},
		}),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	require.Len(t, arr, 3)

	for i, item := range arr {
		m, ok := item.(map[string]any)
		require.True(t, ok, "item %d should be map", i)
		assert.Contains(t, m, "id")
		assert.Contains(t, m, "name")
	}
}

func TestGenerateArrayMinItems(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("integer")},
		MinItems:  ptr(5),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	assert.GreaterOrEqual(t, len(arr), 5)
}

func TestGenerateArrayMaxItems(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("integer")},
		MaxItems:  ptr(2),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	assert.LessOrEqual(t, len(arr), 2)
}

func TestGenerateArrayMinMaxItems(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
		MinItems:  ptr(2),
		MaxItems:  ptr(4),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	assert.GreaterOrEqual(t, len(arr), 2)
	assert.LessOrEqual(t, len(arr), 4)
}

func TestGenerateArrayUniqueItems(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		Items2020: &jsonschema.Schema{
			Types:   newTypes("integer"),
			Minimum: newRat(0),
			Maximum: newRat(1000),
		},
		MinItems:    ptr(5),
		UniqueItems: true,
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	require.GreaterOrEqual(t, len(arr), 5)

	seen := make(map[any]bool, len(arr))
	for _, item := range arr {
		assert.False(t, seen[item], "duplicate item found: %v", item)
		seen[item] = true
	}
}

func TestGenerateArrayNoItemsSchema(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok)
	assert.Empty(t, arr)
}

func TestGenerateArrayDraft07Items(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	itemSchema := &jsonschema.Schema{Types: newTypes("string")}
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		Items: itemSchema, // draft-07 style: *Schema in Items field
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3) // default length

	for _, item := range arr {
		assert.IsType(t, "", item)
	}
}

func TestGenerateArrayPerItemSubSeeding(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
		MinItems:  ptr(3),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	require.Len(t, arr, 3)

	// Items should vary (different sub-seeds produce different values).
	allSame := true

	for i := 1; i < len(arr); i++ {
		if arr[i] != arr[0] {
			allSame = false

			break
		}
	}

	assert.False(t, allSame, "array items should vary across indices")
}

func TestGenerateArrayDeterminism(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("integer")},
	}

	val1, err := g.Generate(schema, 77)
	require.NoError(t, err)

	val2, err := g.Generate(schema, 77)
	require.NoError(t, err)

	assert.Equal(t, val1, val2)
}

// --- Polymorphism tests (8.4.4–8.4.5) ---

func TestGenerateAllOfMergesProperties(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"firstName": {Types: newTypes("string")},
			}),
			objectSchema(map[string]*jsonschema.Schema{
				"lastName": {Types: newTypes("string")},
			}),
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, m, "firstName")
	assert.Contains(t, m, "lastName")
}

func TestGenerateAllOfRequiredUnion(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			{
				Types:      newTypes("object"),
				Properties: map[string]*jsonschema.Schema{"a": {Types: newTypes("string")}},
				Required:   []string{"a"},
			},
			{
				Types:      newTypes("object"),
				Properties: map[string]*jsonschema.Schema{"b": {Types: newTypes("integer")}},
				Required:   []string{"b"},
			},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	assert.Contains(t, m, "a")
	assert.Contains(t, m, "b")
}

func TestGenerateAllOfLastWriteWins(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"x": {Types: newTypes("string")},
			}),
			objectSchema(map[string]*jsonschema.Schema{
				"x": {Types: newTypes("integer")},
			}),
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	// Second schema (integer) should win.
	assert.IsType(t, int64(0), m["x"])
}

func TestGenerateAllOfWithTopLevelProperties(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("object"),
		Properties: map[string]*jsonschema.Schema{
			"id": {Types: newTypes("integer")},
		},
		AllOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"name": {Types: newTypes("string")},
			}),
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	assert.Contains(t, m, "id", "top-level property")
	assert.Contains(t, m, "name", "allOf property")
}

func TestGenerateOneOfBranchSelection(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"catName": {Types: newTypes("string")},
			}),
			objectSchema(map[string]*jsonschema.Schema{
				"dogName": {Types: newTypes("string")},
			}),
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)

	// Should have exactly one branch's properties.
	hasCat := m["catName"] != nil
	hasDog := m["dogName"] != nil
	assert.True(t, hasCat || hasDog, "should have at least one branch")
	assert.False(t, hasCat && hasDog, "should not have both branches")
}

func TestGenerateOneOfDeterminism(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{"a": {Types: newTypes("string")}}),
			objectSchema(map[string]*jsonschema.Schema{"b": {Types: newTypes("string")}}),
			objectSchema(map[string]*jsonschema.Schema{"c": {Types: newTypes("string")}}),
		},
	}

	val1, err := g.Generate(schema, 42)
	require.NoError(t, err)

	val2, err := g.Generate(schema, 42)
	require.NoError(t, err)

	assert.Equal(t, val1, val2, "same seed must select same branch")
}

func TestGenerateOneOfVariation(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{"a": {Types: newTypes("string")}}),
			objectSchema(map[string]*jsonschema.Schema{"b": {Types: newTypes("string")}}),
			objectSchema(map[string]*jsonschema.Schema{"c": {Types: newTypes("string")}}),
		},
	}

	// Try multiple seeds; at least two should produce different branches.
	branches := make(map[string]bool)

	for seed := int64(0); seed < 20; seed++ {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err)

		m, ok := val.(map[string]any)
		require.True(t, ok)

		for k := range m {
			branches[k] = true
		}
	}

	assert.Greater(t, len(branches), 1, "different seeds should select different branches")
}

func TestGenerateAnyOfBranchSelection(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Types: newTypes("string")},
			{Types: newTypes("integer")},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	isString := false
	isInt := false

	switch val.(type) {
	case string:
		isString = true
	case int64:
		isInt = true
	}

	assert.True(t, isString || isInt, "should produce string or integer")
}

func TestGenerateAnyOfDeterminism(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Types: newTypes("string")},
			{Types: newTypes("integer")},
		},
	}

	val1, err := g.Generate(schema, 42)
	require.NoError(t, err)

	val2, err := g.Generate(schema, 42)
	require.NoError(t, err)

	assert.Equal(t, val1, val2)
}

func TestGenerateAllOfWithNestedOneOf(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"id": {Types: newTypes("integer")},
			}),
			{
				OneOf: []*jsonschema.Schema{
					objectSchema(map[string]*jsonschema.Schema{
						"type_a": {Types: newTypes("string")},
					}),
					objectSchema(map[string]*jsonschema.Schema{
						"type_b": {Types: newTypes("string")},
					}),
				},
			},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	assert.Contains(t, m, "id", "allOf base property must be present")

	// Should have one of the oneOf branches.
	hasA := m["type_a"] != nil
	hasB := m["type_b"] != nil
	assert.True(t, hasA || hasB, "should have one branch from nested oneOf")
}

func TestGenerateEmptyAllOf(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	// Empty allOf slice is effectively a no-op — schema falls through to
	// default type resolution (string). This matches JSON Schema semantics:
	// allOf with zero sub-schemas imposes no constraints.
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)
	assert.IsType(t, "", val, "empty allOf falls through to default string")
}

func TestGenerateEmptyOneOf(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	// Empty oneOf slice — same reasoning as empty allOf.
	schema := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)
	assert.IsType(t, "", val, "empty oneOf falls through to default string")
}

// --- Circular reference tests (8.4.6–8.4.7) ---

func TestGenerateSelfReferencingObject(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil) // default maxDepth = 3

	// Build self-referencing schema: TreeNode { name: string, children: [TreeNode] }
	node := &jsonschema.Schema{
		Types: newTypes("object"),
	}
	node.Properties = map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"children": {
			Types:     newTypes("array"),
			Items2020: node, // circular reference
		},
	}

	val, err := g.Generate(node, 42)
	require.NoError(t, err)

	// Should produce a tree that terminates.
	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, m, "name")
	assert.Contains(t, m, "children")
}

func TestGenerateDepth1Termination(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 1, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
		"child": objectSchema(map[string]*jsonschema.Schema{
			"inner": {Types: newTypes("string")},
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	assert.Contains(t, m, "name")
	// At depth 1, child object should be terminated (nil).
	assert.Nil(t, m["child"], "nested object at depth limit should be nil")
}

func TestGenerateDefaultDepth3(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil) // default = 3

	// Build 4-level nesting: L0 → L1 → L2 → L3.
	schema := objectSchema(map[string]*jsonschema.Schema{
		"l1": objectSchema(map[string]*jsonschema.Schema{
			"l2": objectSchema(map[string]*jsonschema.Schema{
				"l3": objectSchema(map[string]*jsonschema.Schema{
					"deep": {Types: newTypes("string")},
				}),
			}),
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")

	// L1 should be populated.
	l1, ok := m["l1"].(map[string]any)
	require.True(t, ok, "l1 should be a map")

	// L2 should be populated.
	l2, ok := l1["l2"].(map[string]any)
	require.True(t, ok, "l2 should be a map")

	// L3 should be terminated (nil) at depth 3.
	assert.Nil(t, l2["l3"], "l3 should be nil at depth limit")
}

func TestGenerateArrayAtDepthLimit(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 1, nil)
	schema := objectSchema(map[string]*jsonschema.Schema{
		"items": {
			Types:     newTypes("array"),
			Items2020: &jsonschema.Schema{Types: newTypes("string")},
		},
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	arr, ok := m["items"].([]any)
	require.True(t, ok, "array at depth limit should be empty []any")
	assert.Empty(t, arr)
}

func TestGenerateCustomMaxDepth(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 5, nil)

	// 5-level nesting should all be populated.
	schema := objectSchema(map[string]*jsonschema.Schema{
		"l1": objectSchema(map[string]*jsonschema.Schema{
			"l2": objectSchema(map[string]*jsonschema.Schema{
				"l3": objectSchema(map[string]*jsonschema.Schema{
					"l4": objectSchema(map[string]*jsonschema.Schema{
						"l5": objectSchema(map[string]*jsonschema.Schema{
							"deep": {Types: newTypes("string")},
						}),
					}),
				}),
			}),
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	l1, ok := m["l1"].(map[string]any)
	require.True(t, ok, "l1 should be map")

	l2, ok := l1["l2"].(map[string]any)
	require.True(t, ok, "l2 should be map")

	l3, ok := l2["l3"].(map[string]any)
	require.True(t, ok, "l3 should be map")

	l4, ok := l3["l4"].(map[string]any)
	require.True(t, ok, "l4 should be map")

	// l5 should be nil at depth 5.
	assert.Nil(t, l4["l5"], "l5 should be nil at depth limit 5")
}

func TestGenerateNonCircularStillLimited(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 2, nil)

	// 3 levels of non-circular nesting, maxDepth 2.
	schema := objectSchema(map[string]*jsonschema.Schema{
		"a": objectSchema(map[string]*jsonschema.Schema{
			"b": objectSchema(map[string]*jsonschema.Schema{
				"c": {Types: newTypes("string")},
			}),
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	a, ok := m["a"].(map[string]any)
	require.True(t, ok, "a should be map")
	assert.Nil(t, a["b"], "non-circular nesting still limited by maxDepth")
}

// --- Null handling tests ---

func TestGenerateNullableObject(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("object", "null"),
		Properties: map[string]*jsonschema.Schema{
			"name": {Types: newTypes("string")},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	// Should generate the object, not nil.
	m, ok := val.(map[string]any)
	require.True(t, ok, "nullable object should generate as object")
	assert.Contains(t, m, "name")
}

func TestGenerateNullableArray(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:     newTypes("array", "null"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "nullable array should generate as array")
	assert.NotEmpty(t, arr)
}

func TestGeneratePureNull(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("null"),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)
	assert.Nil(t, val)
}

// --- Integration tests (8.4.8) ---

func TestGenerateNestedObjectWithSemanticMapping(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewDataGenerator(sm, 0, nil)

	schema := objectSchema(map[string]*jsonschema.Schema{
		"user": objectSchema(map[string]*jsonschema.Schema{
			"email": {Types: newTypes("string")},
			"age":   {Types: newTypes("integer")},
		}),
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")
	user, ok := m["user"].(map[string]any)
	require.True(t, ok, "user should be map")

	// Semantic mapper should produce an email-like value.
	email, ok := user["email"].(string)
	require.True(t, ok)
	assert.Contains(t, email, "@", "email field should get semantic email value")

	// Semantic mapper may return int (via gofakeit) rather than int64.
	// Both are type-compatible with schema type "integer".
	switch user["age"].(type) {
	case int, int64:
		// OK — both are valid integer representations.
	default:
		t.Errorf("age should be integer type, got %T", user["age"])
	}
}

func TestGenerateArrayOfObjectsWithSemanticFields(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewDataGenerator(sm, 0, nil)

	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		Items2020: objectSchema(map[string]*jsonschema.Schema{
			"id":   {Types: newTypes("integer")},
			"name": {Types: newTypes("string")},
		}),
		MinItems: ptr(3),
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	require.Len(t, arr, 3)

	for i, item := range arr {
		m, ok := item.(map[string]any)
		require.True(t, ok, "item %d should be map", i)
		assert.Contains(t, m, "id", "item %d must have id", i)
		assert.Contains(t, m, "name", "item %d must have name", i)
	}

	// Items should vary (different seeds per index).
	m0, ok := arr[0].(map[string]any)
	require.True(t, ok)

	m1, ok := arr[1].(map[string]any)
	require.True(t, ok)
	assert.NotEqual(t, m0["id"], m1["id"], "different items should have different ids")
}

func TestGenerateRealisticPetSchema(t *testing.T) {
	t.Parallel()

	sm := NewSemanticMapper()
	g := NewDataGenerator(sm, 0, nil)

	schema := objectSchema(map[string]*jsonschema.Schema{
		"id":   {Types: newTypes("integer")},
		"name": {Types: newTypes("string")},
		"tag":  {Types: newTypes("string")},
		"address": objectSchema(map[string]*jsonschema.Schema{
			"city":    {Types: newTypes("string")},
			"country": {Types: newTypes("string")},
		}),
		"tags": {
			Types:     newTypes("array"),
			Items2020: &jsonschema.Schema{Types: newTypes("string")},
			MaxItems:  ptr(3),
		},
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any")

	// All top-level fields present.
	assert.Contains(t, m, "id")
	assert.Contains(t, m, "name")
	assert.Contains(t, m, "tag")
	assert.Contains(t, m, "address")
	assert.Contains(t, m, "tags")

	// Nested address.
	addr, ok := m["address"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, addr, "city")

	// Tags array.
	tags, ok := m["tags"].([]any)
	require.True(t, ok)
	assert.LessOrEqual(t, len(tags), 3)

	// Determinism.
	val2, err := g.Generate(schema, 42)
	require.NoError(t, err)
	assert.Equal(t, val, val2)
}

func TestGenerateRefFollowing(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Target schema (what the $ref points to).
	target := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
	})

	// Schema with Ref set (simulating a resolved $ref).
	schema := &jsonschema.Schema{
		Ref: target,
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, m, "name")
}

func TestGeneratePrimitiveThroughDataGenerator(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// String schema.
	val, err := g.Generate(&jsonschema.Schema{Types: newTypes("string")}, 42)
	require.NoError(t, err)
	assert.IsType(t, "", val)

	// Integer schema.
	val, err = g.Generate(&jsonschema.Schema{Types: newTypes("integer")}, 42)
	require.NoError(t, err)
	assert.IsType(t, int64(0), val)

	// Boolean schema.
	val, err = g.Generate(&jsonschema.Schema{Types: newTypes("boolean")}, 42)
	require.NoError(t, err)
	assert.IsType(t, true, val)
}

// --- Review regression tests ---

// B1 regression: circular allOf/oneOf must terminate, not stack overflow.
// With depth-neutral composition, the maxRecursion hard limit (64 calls)
// catches purely circular allOf chains that never enter an object/array.
func TestGenerateCircularAllOfTerminates(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// SchemaA.allOf → SchemaB, SchemaB.allOf → SchemaA (mutual cycle via $ref).
	schemaA := &jsonschema.Schema{}
	schemaB := &jsonschema.Schema{}

	schemaA.AllOf = []*jsonschema.Schema{{Ref: schemaB}}
	schemaB.AllOf = []*jsonschema.Schema{{Ref: schemaA}}

	// Must terminate (not panic with stack overflow).
	val, err := g.Generate(schemaA, 42)
	require.NoError(t, err)

	// At depth limit, allOf with no resolvable properties produces empty map.
	_ = val // any result is acceptable — the test is that it terminates.
}

func TestGenerateCircularOneOfTerminates(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Self-referencing oneOf: Discount = oneOf [ {props: {name}}, $ref Discount ]
	discount := &jsonschema.Schema{}
	discount.OneOf = []*jsonschema.Schema{
		objectSchema(map[string]*jsonschema.Schema{
			"name": {Types: newTypes("string")},
		}),
		{Ref: discount},
	}

	val, err := g.Generate(discount, 42)
	require.NoError(t, err)
	assert.NotNil(t, val, "should produce a value, not nil")
}

// B2 regression: sibling oneOf blocks within separate allOf entries must
// be able to select different branches (not correlated by shared seed).
func TestGenerateAllOfSiblingOneOfIndependentSeeds(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		AllOf: []*jsonschema.Schema{
			{
				OneOf: []*jsonschema.Schema{
					objectSchema(map[string]*jsonschema.Schema{"v1a": {Types: newTypes("string")}}),
					objectSchema(map[string]*jsonschema.Schema{"v1b": {Types: newTypes("string")}}),
				},
			},
			{
				OneOf: []*jsonschema.Schema{
					objectSchema(map[string]*jsonschema.Schema{"v2a": {Types: newTypes("string")}}),
					objectSchema(map[string]*jsonschema.Schema{"v2b": {Types: newTypes("string")}}),
				},
			},
		},
	}

	// Try multiple seeds. With independent sub-schema seeds, the two oneOf
	// blocks should sometimes select different indices.
	sawDifferentBranches := false

	for seed := int64(0); seed < 30; seed++ {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err)

		m, ok := val.(map[string]any)
		require.True(t, ok)

		// Check which branch was selected for each oneOf.
		first := m["v1a"] != nil  // index 0 selected for first oneOf
		second := m["v2a"] != nil // index 0 selected for second oneOf

		if first != second {
			sawDifferentBranches = true

			break
		}
	}

	assert.True(t, sawDifferentBranches,
		"sibling oneOf blocks should be able to select different branches")
}

// NB4: tuple arrays (prefixItems) should produce per-position typed items.
func TestGenerateTupleArray(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		PrefixItems: []*jsonschema.Schema{
			{Types: newTypes("string")},
			{Types: newTypes("integer")},
			{Types: newTypes("boolean")},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any")
	require.Len(t, arr, 3, "tuple should have one item per prefixItems schema")

	assert.IsType(t, "", arr[0], "position 0 should be string")
	assert.IsType(t, int64(0), arr[1], "position 1 should be integer")
	assert.IsType(t, true, arr[2], "position 2 should be boolean")
}

func TestGenerateTupleArrayDeterminism(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types: newTypes("array"),
		PrefixItems: []*jsonschema.Schema{
			{Types: newTypes("string")},
			{Types: newTypes("number")},
		},
	}

	val1, err := g.Generate(schema, 42)
	require.NoError(t, err)

	val2, err := g.Generate(schema, 42)
	require.NoError(t, err)

	assert.Equal(t, val1, val2, "tuple generation should be deterministic")
}

// NB6: boolean additionalProperties should produce empty object.
func TestGenerateAdditionalPropertiesFalse(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:                newTypes("object"),
		AdditionalProperties: false,
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, m, "additionalProperties: false with no properties → empty object")
}

func TestGenerateAdditionalPropertiesTrue(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)
	schema := &jsonschema.Schema{
		Types:                newTypes("object"),
		AdditionalProperties: true,
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, m,
		"additionalProperties: true with no schema → empty object (nothing to generate)")
}

// --- Nullable preference tests (Task 37) ---

func TestIsNullBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema *jsonschema.Schema
		want   bool
	}{
		{
			name:   "null type",
			schema: &jsonschema.Schema{Types: newTypes("null")},
			want:   true,
		},
		{
			name:   "string type",
			schema: &jsonschema.Schema{Types: newTypes("string")},
			want:   false,
		},
		{
			name:   "object type",
			schema: &jsonschema.Schema{Types: newTypes("object")},
			want:   false,
		},
		{
			name:   "nullable inline (object+null) is not a null branch",
			schema: &jsonschema.Schema{Types: newTypes("object", "null")},
			want:   false,
		},
		{
			name:   "nil schema",
			schema: nil,
			want:   false,
		},
		{
			name:   "no types",
			schema: &jsonschema.Schema{},
			want:   false,
		},
		{
			name: "$ref to null type",
			schema: &jsonschema.Schema{
				Ref: &jsonschema.Schema{Types: newTypes("null")},
			},
			want: true,
		},
		{
			name: "$ref to non-null type",
			schema: &jsonschema.Schema{
				Ref: &jsonschema.Schema{Types: newTypes("string")},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isNullBranch(tt.schema))
		})
	}
}

func TestNonNullBranches(t *testing.T) {
	t.Parallel()

	nullSchema := &jsonschema.Schema{Types: newTypes("null")}
	stringSchema := &jsonschema.Schema{Types: newTypes("string")}
	intSchema := &jsonschema.Schema{Types: newTypes("integer")}

	tests := []struct {
		name    string
		schemas []*jsonschema.Schema
		wantLen int
		wantNil bool
	}{
		{
			name:    "mixed: one non-null, one null",
			schemas: []*jsonschema.Schema{stringSchema, nullSchema},
			wantLen: 1,
		},
		{
			name:    "all null",
			schemas: []*jsonschema.Schema{nullSchema, nullSchema},
			wantNil: true,
		},
		{
			name:    "no null branches",
			schemas: []*jsonschema.Schema{stringSchema, intSchema},
			wantLen: 2,
		},
		{
			name:    "single null",
			schemas: []*jsonschema.Schema{nullSchema},
			wantNil: true,
		},
		{
			name:    "single non-null",
			schemas: []*jsonschema.Schema{stringSchema},
			wantLen: 1,
		},
		{
			name:    "three-way: two non-null, one null",
			schemas: []*jsonschema.Schema{stringSchema, intSchema, nullSchema},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := nonNullBranches(tt.schemas)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.Len(t, result, tt.wantLen)

				for _, s := range result {
					assert.False(t, isNullBranch(s), "filtered result should not contain null branches")
				}
			}
		})
	}
}

func TestGenerateBranch_PrefersNonNull(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// anyOf: [{type: object, properties: {name: string}}, {type: null}]
	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"name": {Types: newTypes("string")},
			}),
			{Types: newTypes("null")},
		},
	}

	// Try multiple seeds — should always generate the object, never null.
	for _, seed := range []int64{1, 2, 3, 42, 100, 999, 12345} {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err, "seed=%d", seed)

		m, ok := val.(map[string]any)
		require.True(t, ok, "seed=%d: expected object, got %T (%v)", seed, val, val)
		assert.Contains(t, m, "name", "seed=%d", seed)
	}
}

func TestGenerateBranch_OneOfPrefersNonNull(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// oneOf: [{type: object, properties: {name: string}}, {type: null}]
	// This is the native OpenAPI 3.1 nullable pattern (not compiler-normalized).
	// Must behave identically to the anyOf variant tested above.
	schema := &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			objectSchema(map[string]*jsonschema.Schema{
				"name": {Types: newTypes("string")},
			}),
			{Types: newTypes("null")},
		},
	}

	for _, seed := range []int64{1, 2, 3, 42, 100, 999} {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err, "seed=%d", seed)

		m, ok := val.(map[string]any)
		require.True(t, ok, "seed=%d: expected object, got %T (%v)", seed, val, val)
		assert.Contains(t, m, "name", "seed=%d", seed)
	}
}

func TestGenerateBranch_PureNull(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// anyOf with only null branches — must still generate null.
	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Types: newTypes("null")},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)
	assert.Nil(t, val, "anyOf with only null branches should produce nil")
}

func TestGenerateBranch_NoNullUnchanged(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// anyOf with two non-null branches — seed-based selection should work normally.
	branchA := objectSchema(map[string]*jsonschema.Schema{
		"type_a": {Types: newTypes("string")},
	})
	branchB := objectSchema(map[string]*jsonschema.Schema{
		"type_b": {Types: newTypes("string")},
	})

	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{branchA, branchB},
	}

	// Over multiple seeds, both branches should appear.
	sawA, sawB := false, false

	for seed := int64(0); seed < 20; seed++ {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err)

		m, ok := val.(map[string]any)
		require.True(t, ok)

		if _, has := m["type_a"]; has {
			sawA = true
		}

		if _, has := m["type_b"]; has {
			sawB = true
		}
	}

	assert.True(t, sawA && sawB,
		"both branches should be reachable when no null branch exists (sawA=%v, sawB=%v)", sawA, sawB)
}

func TestGenerateBranch_ThreeWayNullable(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// anyOf: [string, integer, null] → should never pick null, seed picks between string/integer.
	schema := &jsonschema.Schema{
		AnyOf: []*jsonschema.Schema{
			{Types: newTypes("string")},
			{Types: newTypes("integer")},
			{Types: newTypes("null")},
		},
	}

	sawString, sawInt := false, false

	for seed := int64(0); seed < 20; seed++ {
		val, err := g.Generate(schema, seed)
		require.NoError(t, err)
		require.NotNil(t, val, "seed=%d: should never generate null", seed)

		switch val.(type) {
		case string:
			sawString = true
		case int64:
			sawInt = true
		default:
			t.Fatalf("seed=%d: unexpected type %T", seed, val)
		}
	}

	assert.True(t, sawString && sawInt,
		"both non-null branches should be reachable (sawString=%v, sawInt=%v)", sawString, sawInt)
}

// --- Level 2: Object/array schema examples (24.4) ---

func TestGenerateObjectWithSchemaExample(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Object schema with an example — should return the example directly.
	schema := &jsonschema.Schema{
		Types: newTypes("object"),
		Properties: map[string]*jsonschema.Schema{
			"name": {Types: newTypes("string")},
			"age":  {Types: newTypes("integer")},
		},
		Examples: []any{
			map[string]any{"name": "Fido", "age": 3},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok, "expected map[string]any, got %T", val)
	assert.Equal(t, "Fido", m["name"])
	assert.Equal(t, 3, m["age"])
}

func TestGenerateArrayWithSchemaExample(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Array schema with an example — should return the example directly.
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
		Examples: []any{
			[]any{"alpha", "beta", "gamma"},
		},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok, "expected []any, got %T", val)
	assert.Equal(t, []any{"alpha", "beta", "gamma"}, arr)
}

func TestGenerateObjectWithoutSchemaExample(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Object schema without examples — should fall through to generateObject.
	schema := objectSchema(map[string]*jsonschema.Schema{
		"name": {Types: newTypes("string")},
	})

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, m, "name", "should generate properties normally")
}

func TestGenerateArrayWithoutSchemaExample(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Array without examples — should fall through to generateArray.
	schema := &jsonschema.Schema{
		Types:     newTypes("array"),
		Items2020: &jsonschema.Schema{Types: newTypes("string")},
	}

	val, err := g.Generate(schema, 42)
	require.NoError(t, err)

	arr, ok := val.([]any)
	require.True(t, ok)
	assert.NotEmpty(t, arr, "should generate items normally")
}

func TestGenerateNestedObjectChildHasExample(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	// Parent has no example, child property has an object-level example.
	childSchema := &jsonschema.Schema{
		Types: newTypes("object"),
		Properties: map[string]*jsonschema.Schema{
			"city": {Types: newTypes("string")},
			"zip":  {Types: newTypes("string")},
		},
		Examples: []any{
			map[string]any{"city": "Portland", "zip": "97201"},
		},
	}

	parentSchema := objectSchema(map[string]*jsonschema.Schema{
		"name":    {Types: newTypes("string")},
		"address": childSchema,
	})

	val, err := g.Generate(parentSchema, 42)
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)

	// Parent's "name" should be generated normally.
	assert.Contains(t, m, "name")
	assert.IsType(t, "", m["name"])

	// Child "address" should be the example value.
	addr, ok := m["address"].(map[string]any)
	require.True(t, ok, "address should be map[string]any")
	assert.Equal(t, "Portland", addr["city"])
	assert.Equal(t, "97201", addr["zip"])
}

func TestGenerateObjectExampleDeterministic(t *testing.T) {
	t.Parallel()

	g := NewDataGenerator(nil, 0, nil)

	schema := &jsonschema.Schema{
		Types: newTypes("object"),
		Properties: map[string]*jsonschema.Schema{
			"id": {Types: newTypes("integer")},
		},
		Examples: []any{
			map[string]any{"id": 42},
		},
	}

	// Same input → same output.
	val1, err1 := g.Generate(schema, 100)
	require.NoError(t, err1)

	val2, err2 := g.Generate(schema, 100)
	require.NoError(t, err2)
	assert.Equal(t, val1, val2, "object example should be deterministic")
}
