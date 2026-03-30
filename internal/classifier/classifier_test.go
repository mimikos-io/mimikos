package classifier

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/stretchr/testify/assert"
)

func newClassifier() *Classifier {
	return New()
}

func op(method, path string) parser.Operation {
	return parser.Operation{Method: method, Path: path}
}

// --- Layer 1: GET requests ---

func TestClassify_GET_Collection_IsList(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"simple collection", "/pets"},
		{"nested collection", "/users/{userId}/orders"},
		{"browse subcollection", "/browse/categories"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("GET", tt.path))
			assert.Equal(t, model.BehaviorList, result.Type)
			assert.InDelta(t, 0.8, result.Confidence, 0.01)
		})
	}
}

func TestClassify_GET_Item_IsFetch(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"simple item", "/pets/{petId}"},
		{"nested item", "/users/{userId}/orders/{orderId}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("GET", tt.path))
			assert.Equal(t, model.BehaviorFetch, result.Type)
			assert.InDelta(t, 0.8, result.Confidence, 0.01)
		})
	}
}

// --- Layer 1: POST requests ---

func TestClassify_POST_Collection_IsCreate(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"simple collection", "/pets"},
		{"nested collection", "/users/{userId}/playlists"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("POST", tt.path))
			assert.Equal(t, model.BehaviorCreate, result.Type)
			assert.InDelta(t, 0.8, result.Confidence, 0.01)
		})
	}
}

func TestClassify_POST_Item_IsGeneric(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"post to item", "/customers/{id}"},
		{"nested post to item", "/users/{userId}/orders/{orderId}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("POST", tt.path))
			assert.Equal(t, model.BehaviorGeneric, result.Type)
			assert.InDelta(t, 0.4, result.Confidence, 0.01)
		})
	}
}

func TestClassify_POST_ActionVerb_IsGeneric(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"capture charge", "/charges/{id}/capture"},
		{"merge pull request", "/pulls/{number}/merge"},
		{"search endpoint", "/search"},
		{"query database", "/databases/{id}/query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("POST", tt.path))
			assert.Equal(t, model.BehaviorGeneric, result.Type)
		})
	}
}

// --- Layer 1: PUT requests ---

func TestClassify_PUT_Item_IsUpdate(t *testing.T) {
	c := newClassifier()

	result := c.Classify(op("PUT", "/playlists/{playlist_id}"))
	assert.Equal(t, model.BehaviorUpdate, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01)
}

func TestClassify_PUT_Collection_IsGeneric(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"bulk save", "/me/albums"},
		{"bulk action", "/me/tracks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("PUT", tt.path))
			assert.Equal(t, model.BehaviorGeneric, result.Type)
			assert.InDelta(t, 0.6, result.Confidence, 0.01)
		})
	}
}

func TestClassify_PUT_ActionVerb_IsGeneric(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"pause playback", "/me/player/pause"},
		{"play music", "/me/player/play"},
		{"seek position", "/me/player/seek"},
		{"shuffle toggle", "/me/player/shuffle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("PUT", tt.path))
			assert.Equal(t, model.BehaviorGeneric, result.Type)
		})
	}
}

// --- Layer 1: PATCH requests ---

func TestClassify_PATCH_Item_IsUpdate(t *testing.T) {
	c := newClassifier()

	result := c.Classify(op("PATCH", "/pets/{petId}"))
	assert.Equal(t, model.BehaviorUpdate, result.Type)
	assert.InDelta(t, 0.8, result.Confidence, 0.01)
}

func TestClassify_PATCH_Collection_IsGeneric(t *testing.T) {
	c := newClassifier()

	result := c.Classify(op("PATCH", "/bulk/users"))
	assert.Equal(t, model.BehaviorGeneric, result.Type)
	assert.InDelta(t, 0.6, result.Confidence, 0.01)
}

// --- Layer 1: DELETE requests ---

func TestClassify_DELETE_Item_IsDelete(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"simple item", "/pets/{petId}"},
		{"nested item", "/users/{userId}/orders/{orderId}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("DELETE", tt.path))
			assert.Equal(t, model.BehaviorDelete, result.Type)
			assert.InDelta(t, 0.8, result.Confidence, 0.01)
		})
	}
}

func TestClassify_DELETE_Collection_IsGeneric(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name string
		path string
	}{
		{"bulk delete", "/me/albums"},
		{"bulk remove", "/me/episodes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op("DELETE", tt.path))
			assert.Equal(t, model.BehaviorGeneric, result.Type)
			assert.InDelta(t, 0.6, result.Confidence, 0.01)
		})
	}
}

// --- Layer 1: Twilio path normalization ---

func TestClassify_TwilioJSONSuffix(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name     string
		method   string
		path     string
		wantType model.BehaviorType
	}{
		{"list accounts", "GET", "/2010-04-01/Accounts.json", model.BehaviorList},
		{"fetch account", "GET", "/2010-04-01/Accounts/{AccountSid}.json", model.BehaviorFetch},
		{"create call", "POST", "/2010-04-01/Accounts/{AccountSid}/Calls.json", model.BehaviorCreate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.Classify(op(tt.method, tt.path))
			assert.Equal(t, tt.wantType, result.Type)
		})
	}
}

// --- Layer 1: Edge cases ---

func TestClassify_UnknownMethod_IsGeneric(t *testing.T) {
	c := newClassifier()

	result := c.Classify(op("OPTIONS", "/pets"))
	assert.Equal(t, model.BehaviorGeneric, result.Type)
	assert.InDelta(t, 0.4, result.Confidence, 0.01)
}

func TestClassify_EmptyPath_GET_IsList(t *testing.T) {
	c := newClassifier()

	// Empty path is treated as collection, so GET → list.
	result := c.Classify(op("GET", ""))
	assert.Equal(t, model.BehaviorList, result.Type)
}

func TestClassify_RootPath(t *testing.T) {
	c := newClassifier()

	result := c.Classify(op("GET", "/"))
	assert.Equal(t, model.BehaviorList, result.Type)
}

// --- Pipeline interaction tests ---

// TestClassify_Pipeline_L2AndL3_ConfidenceCapping verifies that L2 and L3
// boosts are both applied and capped at 0.9.
func TestClassify_Pipeline_L2AndL3_ConfidenceCapping(t *testing.T) {
	c := newClassifier()

	// POST /users with 201 response and "createUser" operationId.
	// L1: create (0.8) → L2: 201 boost → 0.9 → L3: "create" confirms, capped at 0.9.
	op := parser.Operation{
		Method:      "POST",
		Path:        "/users",
		OperationID: "createUser",
		Responses: map[int]*parser.Response{
			201: {StatusCode: 201},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type)
	assert.InDelta(t, 0.9, result.Confidence, 0.01,
		"L2 + L3 boosts should cap at 0.9")
}

// TestClassify_Pipeline_ActionVerbCRUDKeywordOverlap exercises the interaction
// between actionVerbs (L1) and crudKeywords (L3) for tokens that appear in
// both sets (add, remove, insert, search). L1 detects the action and
// produces generic at moderate confidence (0.6). L3 sees the same token as
// a CRUD keyword but cannot override because confidence > 0.4.
func TestClassify_Pipeline_ActionVerbCRUDKeywordOverlap(t *testing.T) {
	c := newClassifier()

	tests := []struct {
		name        string
		path        string
		operationID string
	}{
		{"add in path and operationId", "/tasks/{id}/addFollowers", "addTaskFollowers"},
		{"remove in path and operationId", "/tasks/{id}/removeFollowers", "removeTaskFollowers"},
		{"insert in path", "/fields/{id}/insert", "insertEnumOption"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "POST",
				Path:        tt.path,
				OperationID: tt.operationID,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorGeneric, result.Type,
				"L1 action verb should produce generic, L3 should not override at moderate confidence")
			assert.InDelta(t, 0.6, result.Confidence, 0.01,
				"confidence stays at moderate (0.6), not overridden by L3")
		})
	}
}

// TestClassify_Pipeline_L3OverrideAtWeakConfidence verifies L3 can override
// a weak L1 result (POST to item → generic 0.4) with an operationId signal.
func TestClassify_Pipeline_L3OverrideAtWeakConfidence(t *testing.T) {
	c := newClassifier()

	op := parser.Operation{
		Method:      "POST",
		Path:        "/customers/{id}",
		OperationID: "updateCustomer",
		Responses: map[int]*parser.Response{
			200: {StatusCode: 200},
		},
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorUpdate, result.Type,
		"L3 overrides weak POST-to-item generic with update keyword")
	assert.InDelta(t, 0.6, result.Confidence, 0.01,
		"L3 override sets confidence to 0.6")
}
