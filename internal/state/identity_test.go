package state

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferResourceIdentity_TopLevelID(t *testing.T) {
	body := map[string]any{"id": "abc", "name": "Fido"}

	resType, resID := InferResourceIdentity("/users", nil, body)
	assert.Equal(t, "users", resType)
	assert.Equal(t, "abc", resID)
}

func TestInferResourceIdentity_TopLevelID_CaseInsensitive(t *testing.T) {
	body := map[string]any{"ID": "upper-123", "name": "Fido"}

	resType, resID := InferResourceIdentity("/pets", nil, body)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "upper-123", resID)
}

func TestInferResourceIdentity_TopLevelID_Integer(t *testing.T) {
	body := map[string]any{"id": float64(42), "name": "Fido"}

	resType, resID := InferResourceIdentity("/pets", nil, body)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "42", resID)
}

func TestInferResourceIdentity_TopLevelID_JSONNumber(t *testing.T) {
	raw := `{"id": 99, "name": "Fido"}`

	var body map[string]any

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&body))

	resType, resID := InferResourceIdentity("/pets", nil, body)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "99", resID)
}

func TestInferResourceIdentity_PathParamFromParams(t *testing.T) {
	params := map[string]string{"id": "abc"}

	resType, resID := InferResourceIdentity("/users/{id}", params, nil)
	assert.Equal(t, "users", resType)
	assert.Equal(t, "abc", resID)
}

func TestInferResourceIdentity_PathParamNameMatchInBody(t *testing.T) {
	body := map[string]any{"petId": float64(42), "name": "Fido"}
	params := map[string]string{"petId": "42"}

	resType, resID := InferResourceIdentity("/pets/{petId}", params, body)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "42", resID)
}

func TestInferResourceIdentity_NestedPath(t *testing.T) {
	params := map[string]string{"userId": "u1", "orderId": "xyz"}

	resType, resID := InferResourceIdentity("/users/{userId}/orders/{orderId}", params, nil)
	assert.Equal(t, "orders", resType)
	assert.Equal(t, "xyz", resID)
}

func TestInferResourceIdentity_NoIDField_NoPathParam_FallbackUUID(t *testing.T) {
	body := map[string]any{"userId": "abc", "name": "Alice"}

	resType, resID := InferResourceIdentity("/users", nil, body)
	assert.Equal(t, "users", resType)
	assert.NotEmpty(t, resID, "should generate fallback UUID")
	assert.Len(t, resID, 36, "UUID format: 8-4-4-4-12 = 36 chars")
}

func TestInferResourceIdentity_NilBody(t *testing.T) {
	resType, resID := InferResourceIdentity("/users", nil, nil)
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "nil body should produce fallback UUID")
}

func TestInferResourceIdentity_NonObjectBody(t *testing.T) {
	resType, resID := InferResourceIdentity("/users", nil, "just a string")
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "non-object body should produce fallback UUID")
}

func TestInferResourceIdentity_ArrayBody(t *testing.T) {
	body := []any{"a", "b", "c"}

	resType, resID := InferResourceIdentity("/users", nil, body)
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "array body should produce fallback UUID")
}

func TestInferResourceIdentity_VersionedPath(t *testing.T) {
	body := map[string]any{"id": "p1"}

	resType, resID := InferResourceIdentity("/v1/pets", nil, body)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "p1", resID)
}

func TestInferResourceIdentity_VersionedPathWithParam(t *testing.T) {
	params := map[string]string{"petId": "p1"}

	resType, resID := InferResourceIdentity("/v2/pets/{petId}", params, nil)
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "p1", resID)
}

func TestInferResourceIdentity_EmptyPath(t *testing.T) {
	resType, resID := InferResourceIdentity("", nil, nil)
	assert.Empty(t, resType)
	assert.Len(t, resID, 36, "empty path should produce fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_Deterministic(t *testing.T) {
	body := map[string]any{"name": "no-id-here"}

	id1Type, id1 := InferResourceIdentity("/users", nil, body)
	id2Type, id2 := InferResourceIdentity("/users", nil, body)

	assert.Equal(t, id1Type, id2Type)
	assert.Equal(t, id1, id2, "same input should produce same fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_VariesByPath(t *testing.T) {
	body := map[string]any{"name": "same-body"}

	_, id1 := InferResourceIdentity("/users", nil, body)
	_, id2 := InferResourceIdentity("/orders", nil, body)

	assert.NotEqual(t, id1, id2, "different paths should produce different UUIDs")
}

func TestInferResourceIdentity_IDPriority_BodyIDOverPathParam(t *testing.T) {
	// When body has a top-level "id" field AND path params exist,
	// the top-level "id" field takes priority.
	body := map[string]any{"id": "body-id", "petId": "body-petId"}
	params := map[string]string{"petId": "param-petId"}

	_, resID := InferResourceIdentity("/pets/{petId}", params, body)
	assert.Equal(t, "body-id", resID)
}

// --- Resource type extraction ---

func TestExtractResourceType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantType string
	}{
		{"simple collection", "/users", "users"},
		{"item path", "/users/{id}", "users"},
		{"nested item", "/users/{userId}/orders/{orderId}", "orders"},
		{"nested collection", "/users/{userId}/orders", "orders"},
		{"versioned", "/v1/pets", "pets"},
		{"versioned with param", "/v2/pets/{petId}", "pets"},
		{"deeply versioned", "/api/v3/pets", "pets"},
		{"root", "/", ""},
		{"empty", "", ""},
		{"param only", "/{id}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantType, extractResourceType(tt.path))
		})
	}
}
