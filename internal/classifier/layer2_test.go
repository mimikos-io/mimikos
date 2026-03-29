package classifier_test

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/stretchr/testify/assert"
)

// --- Layer 2: Array response confirmation ---

func TestLayer2_GET_Collection_ArrayResponse_ConfirmsList(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "GET",
		Path:   "/pets",
		Responses: map[int]*parser.Response{
			200: {
				StatusCode: 200,
				Schema:     &parser.SchemaRef{Raw: &base.Schema{Type: []string{"array"}}},
			},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type)
	assert.InDelta(t, 0.9, result.Confidence, 0.01,
		"array response confirms list: 0.8 + 0.1 = 0.9")
}

func TestLayer2_GET_Collection_ObjectResponse_NoBehaviorChange(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "GET",
		Path:   "/pets",
		Responses: map[int]*parser.Response{
			200: {
				StatusCode: 200,
				Schema:     &parser.SchemaRef{Raw: &base.Schema{Type: []string{"object"}}},
			},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"object response does not confirm list, stays at L1 confidence")
}

func TestLayer2_GET_Collection_NoSchema_NoBehaviorChange(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "GET",
		Path:   "/pets",
		Responses: map[int]*parser.Response{
			200: {StatusCode: 200, Schema: nil},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"nil schema does not trigger array confirmation")
}

func TestLayer2_GET_Item_ArrayResponse_DoesNotAffectFetch(t *testing.T) {
	c := classifier.New()

	// GET item is fetch — array check only applies to list.
	op := parser.Operation{
		Method: "GET",
		Path:   "/pets/{petId}",
		Responses: map[int]*parser.Response{
			200: {
				StatusCode: 200,
				Schema:     &parser.SchemaRef{Raw: &base.Schema{Type: []string{"array"}}},
			},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"array check does not affect non-list types")
}

// --- Layer 2: Status code 201 ---

func TestLayer2_POST_Collection_With201_ConfirmsCreate(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "POST",
		Path:   "/pets",
		Responses: map[int]*parser.Response{
			201: {StatusCode: 201},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type)
	assert.InDelta(t, 0.9, result.Confidence, 0.01,
		"201 confirms create: 0.8 + 0.1 boost = 0.9")
}

func TestLayer2_POST_Collection_Without201_NoChange(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "POST",
		Path:   "/pets",
		Responses: map[int]*parser.Response{
			200: {StatusCode: 200},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"no 201 means no boost, stays at L1 confidence")
}

func TestLayer2_POST_Collection_NoResponses_NoChange(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "POST",
		Path:   "/pets",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"nil responses means no boost")
}

// --- Layer 2: Status code 204 ---

func TestLayer2_DELETE_Item_With204_ConfirmsDelete(t *testing.T) {
	c := classifier.New()
	op := parser.Operation{
		Method: "DELETE",
		Path:   "/pets/{petId}",
		Responses: map[int]*parser.Response{
			204: {StatusCode: 204},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorDelete, result.Type)
	assert.InDelta(t, 0.9, result.Confidence, 0.01,
		"204 confirms delete: 0.8 + 0.1 boost = 0.9")
}

func TestLayer2_Generic_With204_ConfirmsGeneric(t *testing.T) {
	c := classifier.New()

	// POST to action path (generic from L1).
	op := parser.Operation{
		Method: "POST",
		Path:   "/charges/{id}/capture",
		Responses: map[int]*parser.Response{
			204: {StatusCode: 204},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorGeneric, result.Type)
	assert.InDelta(t, 0.7, result.Confidence, 0.01,
		"204 confirms generic action: 0.6 + 0.1 boost = 0.7")
}

func TestLayer2_NonMatchingType_NoBehaviorChange(t *testing.T) {
	c := classifier.New()

	// GET item is fetch — 204 should not affect it (only affects delete/generic).
	op := parser.Operation{
		Method: "GET",
		Path:   "/pets/{petId}",
		Responses: map[int]*parser.Response{
			204: {StatusCode: 204},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01,
		"204 does not affect fetch classification")
}
