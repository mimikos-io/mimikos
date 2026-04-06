package state

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Strategy 1: Exact match (path param name matches body field) ---

func TestInferResourceIdentity_ExactMatch_ID(t *testing.T) {
	body := map[string]any{"id": "1", "name": "Fido"}

	resType, resID := InferResourceIdentity("/pets/{id}", nil, body, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "1", resID)
}

func TestInferResourceIdentity_ExactMatch_CaseInsensitive(t *testing.T) {
	// Twilio pattern: path param {Sid} matches body field "sid".
	body := map[string]any{"sid": "XX123", "name": "Call"}

	resType, resID := InferResourceIdentity("/calls/{Sid}", nil, body, "")
	assert.Equal(t, "calls", resType)
	assert.Equal(t, "XX123", resID)
}

func TestInferResourceIdentity_ExactMatch_Integer(t *testing.T) {
	body := map[string]any{"id": float64(42), "name": "Fido"}

	resType, resID := InferResourceIdentity("/pets/{id}", nil, body, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "42", resID)
}

func TestInferResourceIdentity_ExactMatch_JSONNumber(t *testing.T) {
	raw := `{"id": 99, "name": "Fido"}`

	var body map[string]any

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&body))

	resType, resID := InferResourceIdentity("/pets/{id}", nil, body, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "99", resID)
}

// --- Strategy 2: Suffix strip ---

func TestInferResourceIdentity_SuffixStrip_CamelCase(t *testing.T) {
	// Petstore pattern: {petId} → strip "pet" → "id" → body["id"]
	body := map[string]any{"id": float64(1), "name": "Fido"}

	resType, resID := InferResourceIdentity("/pets/{petId}", nil, body, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "1", resID)
}

func TestInferResourceIdentity_SuffixStrip_Underscore(t *testing.T) {
	// Asana pattern: {task_gid} → strip "task_" → "gid" → body["gid"]
	body := map[string]any{"gid": "abc", "name": "Task"}

	resType, resID := InferResourceIdentity("/tasks/{task_gid}", nil, body, "")
	assert.Equal(t, "tasks", resType)
	assert.Equal(t, "abc", resID)
}

func TestInferResourceIdentity_SuffixStrip_CommentID(t *testing.T) {
	// GitHub pattern: {comment_id} → strip "comment_" → "id" → body["id"]
	body := map[string]any{"id": float64(42), "body": "comment text"}

	resType, resID := InferResourceIdentity("/comments/{comment_id}", nil, body, "")
	assert.Equal(t, "comments", resType)
	assert.Equal(t, "42", resID)
}

func TestInferResourceIdentity_SuffixStrip_CamelCase_NoID(t *testing.T) {
	// {taskGid} → strip "task" → "gid" → body["gid"]
	body := map[string]any{"gid": "abc", "name": "Task"}

	resType, resID := InferResourceIdentity("/tasks/{taskGid}", nil, body, "")
	assert.Equal(t, "tasks", resType)
	assert.Equal(t, "abc", resID)
}

// --- Strategy 3: ID field hint ---

func TestInferResourceIdentity_IDFieldHint(t *testing.T) {
	// Create operation with no path param, hint=gid from fetch variant.
	body := map[string]any{"gid": "abc", "name": "Project"}

	resType, resID := InferResourceIdentity("/projects", nil, body, "gid")
	assert.Equal(t, "projects", resType)
	assert.Equal(t, "abc", resID)
}

// --- Strategy 4: Body "id" fallback ---

func TestInferResourceIdentity_BodyIDFallback(t *testing.T) {
	// Stripe pattern: {customer} doesn't suffix-strip to a body field,
	// but body has "id".
	body := map[string]any{"id": "cus_123", "name": "Customer"}

	_, resID := InferResourceIdentity("/customers/{customer}", nil, body, "")
	assert.Equal(t, "cus_123", resID)
}

func TestInferResourceIdentity_BodyIDFallback_CaseInsensitive(t *testing.T) {
	body := map[string]any{"ID": "upper-123", "name": "Fido"}

	_, resID := InferResourceIdentity("/pets", nil, body, "")
	assert.Equal(t, "upper-123", resID)
}

func TestInferResourceIdentity_BodyIDFallback_NoPathParam(t *testing.T) {
	// Collection path, no param, body has id.
	body := map[string]any{"id": "abc", "name": "Fido"}

	resType, resID := InferResourceIdentity("/users", nil, body, "")
	assert.Equal(t, "users", resType)
	assert.Equal(t, "abc", resID)
}

// --- Strategy 5: Path param value lookup (for fetch/update/delete) ---

func TestInferResourceIdentity_PathParamFromParams(t *testing.T) {
	params := map[string]string{"id": "abc"}

	resType, resID := InferResourceIdentity("/users/{id}", params, nil, "")
	assert.Equal(t, "users", resType)
	assert.Equal(t, "abc", resID)
}

func TestInferResourceIdentity_PathParamFromParams_Nested(t *testing.T) {
	params := map[string]string{"userId": "u1", "orderId": "xyz"}

	resType, resID := InferResourceIdentity("/users/{userId}/orders/{orderId}", params, nil, "")
	assert.Equal(t, "orders", resType)
	assert.Equal(t, "xyz", resID)
}

func TestInferResourceIdentity_PathParamFromParams_Versioned(t *testing.T) {
	params := map[string]string{"petId": "p1"}

	resType, resID := InferResourceIdentity("/v2/pets/{petId}", params, nil, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "p1", resID)
}

// --- Strategy 6: Deterministic UUID fallback ---

func TestInferResourceIdentity_FallbackUUID_NoMatches(t *testing.T) {
	body := map[string]any{"foo": "bar"}

	resType, resID := InferResourceIdentity("/things/{x}", nil, body, "")
	assert.Equal(t, "things", resType)
	assert.NotEmpty(t, resID)
	assert.Len(t, resID, 36, "UUID format: 8-4-4-4-12 = 36 chars")
}

func TestInferResourceIdentity_FallbackUUID_NilBody(t *testing.T) {
	resType, resID := InferResourceIdentity("/users", nil, nil, "")
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "nil body should produce fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_NonObjectBody(t *testing.T) {
	resType, resID := InferResourceIdentity("/users", nil, "just a string", "")
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "non-object body should produce fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_ArrayBody(t *testing.T) {
	body := []any{"a", "b", "c"}

	resType, resID := InferResourceIdentity("/users", nil, body, "")
	assert.Equal(t, "users", resType)
	assert.Len(t, resID, 36, "array body should produce fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_Deterministic(t *testing.T) {
	body := map[string]any{"name": "no-id-here"}

	_, id1 := InferResourceIdentity("/users", nil, body, "")
	_, id2 := InferResourceIdentity("/users", nil, body, "")

	assert.Equal(t, id1, id2, "same input should produce same fallback UUID")
}

func TestInferResourceIdentity_FallbackUUID_VariesByPath(t *testing.T) {
	body := map[string]any{"name": "same-body"}

	_, id1 := InferResourceIdentity("/users", nil, body, "")
	_, id2 := InferResourceIdentity("/orders", nil, body, "")

	assert.NotEqual(t, id1, id2, "different paths should produce different UUIDs")
}

func TestInferResourceIdentity_EmptyPath(t *testing.T) {
	resType, resID := InferResourceIdentity("", nil, nil, "")
	assert.Empty(t, resType)
	assert.Len(t, resID, 36, "empty path should produce fallback UUID")
}

func TestInferResourceIdentity_VersionedPath(t *testing.T) {
	body := map[string]any{"id": "p1"}

	resType, resID := InferResourceIdentity("/v1/pets", nil, body, "")
	assert.Equal(t, "pets", resType)
	assert.Equal(t, "p1", resID)
}

// --- Priority: exact match in body beats path param value ---

func TestInferResourceIdentity_BodyExactMatch_OverPathParam(t *testing.T) {
	// When body has the exact path param field AND pathParams has a value,
	// the body field wins (Strategy 1 before Strategy 5).
	body := map[string]any{"petId": "body-petId", "id": "body-id"}
	params := map[string]string{"petId": "param-petId"}

	_, resID := InferResourceIdentity("/pets/{petId}", params, body, "")
	assert.Equal(t, "body-petId", resID)
}

// --- singularize tests ---

func TestSingularize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"pets", "pet"},
		{"tasks", "task"},
		{"categories", "category"},
		{"addresses", "address"},
		{"statuses", "status"},
		{"projects", "project"},
		{"customers", "customer"},
		{"", ""},
		{"data", "data"},          // uncountable / already singular
		{"class", "class"},        // ends in "ss" — no strip
		{"playlists", "playlist"}, // standard -s
		{"buses", "bus"},          // -ses → drop "es"
		{"boxes", "box"},          // -xes → drop "es"
		{"quizzes", "quizz"},      // -zes → drop "es" (not perfect, but functional)
		{"matches", "match"},      // -ches → drop "es"
		{"crashes", "crash"},      // -shes → drop "es"
		{"replies", "reply"},      // -ies → -y
		{"messages", "message"},   // -ges → drop "s" (not -ses pattern)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, Singularize(tt.input))
		})
	}
}

// --- stripResourcePrefix tests ---

func TestStripResourcePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		param    string
		resource string
		want     string
	}{
		{"petId", "pets", "id"},
		{"task_gid", "tasks", "gid"},
		{"comment_id", "comments", "id"},
		{"id", "pets", ""},
		{"customer", "customers", ""},
		{"taskGid", "tasks", "gid"},
		{"project_gid", "projects", "gid"},
		{"liveStreamId", "live_streams", ""}, // singular doesn't match — no prefix
		{"userId", "users", "id"},
		{"", "pets", ""},
		{"petId", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.param+"_"+tt.resource, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, StripResourcePrefix(tt.param, tt.resource))
		})
	}
}

// --- findField tests ---

func TestFindField(t *testing.T) {
	t.Parallel()

	body := map[string]any{"id": "123", "Name": "Alice", "gid": "abc"}

	t.Run("exact match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "123", findField(body, "id"))
	})

	t.Run("case-insensitive match", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Alice", findField(body, "name"))
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, findField(body, "missing"))
	})
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
