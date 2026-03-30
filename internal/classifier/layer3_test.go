package classifier

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/stretchr/testify/assert"
)

// --- Layer 3: operationId tokenization ---

func TestTokenizeOperationID(t *testing.T) {
	tests := []struct {
		name   string
		opID   string
		expect []string
	}{
		{"camelCase", "createUser", []string{"create", "user"}},
		{"PascalCase", "PostAccountsAccount", []string{"post", "accounts", "account"}},
		{"kebab-case", "get-current-users-profile", []string{"get", "current", "users", "profile"}},
		{"snake_case", "delete_user_by_id", []string{"delete", "user", "by", "id"}},
		{"slash separated", "apps/get-authenticated", []string{"apps", "get", "authenticated"}},
		{"mixed", "PostChargesCharge_capture", []string{"post", "charges", "charge", "capture"}},
		{"all caps abbreviation", "listAPIKeys", []string{"list", "api", "keys"}},
		{"empty", "", nil},
		{"single word", "list", []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TokenizeOperationID(tt.opID)
			assert.Equal(t, tt.expect, got)
		})
	}
}

// --- Layer 3: keyword matching on POST-to-item ---

func TestLayer3_POST_Item_UpdateKeyword_OverridesToUpdate(t *testing.T) {
	c := New()

	tests := []struct {
		name        string
		operationID string
	}{
		{"updateCustomer", "updateCustomer"},
		{"edit-user-profile", "edit-user-profile"},
		{"modify_account", "modify_account"},
		{"patchResource", "patchResource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "POST",
				Path:        "/customers/{id}",
				OperationID: tt.operationID,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorUpdate, result.Type,
				"operationId %q should override POST-to-item to update", tt.operationID)
			assert.InDelta(t, 0.6, result.Confidence, 0.01,
				"L3 override confidence should be 0.6")
		})
	}
}

func TestLayer3_POST_Item_ActionKeyword_StaysGeneric(t *testing.T) {
	c := New()

	tests := []struct {
		name        string
		operationID string
	}{
		{"captureCharge", "captureCharge"},
		{"cancel-payment", "cancel-payment"},
		{"merge_pull_request", "merge_pull_request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "POST",
				Path:        "/charges/{id}",
				OperationID: tt.operationID,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorGeneric, result.Type,
				"action operationId %q should keep generic", tt.operationID)
		})
	}
}

func TestLayer3_POST_Item_NoOperationID_StaysGeneric(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method: "POST",
		Path:   "/customers/{id}",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorGeneric, result.Type,
		"no operationId means L3 cannot disambiguate, stays generic")
}

func TestLayer3_POST_Item_NoKeywordMatch_StaysGeneric(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/customers/{id}",
		OperationID: "PostCustomersCustomer",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorGeneric, result.Type,
		"no CRUD keyword in operationId, stays generic")
}

// --- Layer 3: keyword matching on other methods ---

func TestLayer3_POST_Collection_CreateKeyword_ConfirmsCreate(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/users",
		OperationID: "createUser",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type)
	// L1 strong (0.8) + L3 boost (0.05) = 0.85
	assert.InDelta(t, 0.85, result.Confidence, 0.01)
}

func TestLayer3_GET_Collection_ListKeyword_ConfirmsList(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "GET",
		Path:        "/orders",
		OperationID: "listOrders",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type)
	// L1 strong (0.8) + L3 boost (0.05) = 0.85
	assert.InDelta(t, 0.85, result.Confidence, 0.01)
}

func TestLayer3_GET_Item_FetchKeyword_ConfirmsFetch(t *testing.T) {
	c := New()
	// "fetchUserById" — "fetch" is a CRUD keyword distinct from HTTP method "GET".
	op := parser.Operation{
		Method:      "GET",
		Path:        "/users/{userId}",
		OperationID: "fetchUserById",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type)
	// L1 strong (0.8) + L3 boost (0.05) = 0.85
	assert.InDelta(t, 0.85, result.Confidence, 0.01)
}

func TestLayer3_DELETE_Item_DeleteKeyword_ConfirmsDelete(t *testing.T) {
	c := New()
	// "removeItem" — "remove" is a CRUD keyword distinct from HTTP method "DELETE".
	op := parser.Operation{
		Method:      "DELETE",
		Path:        "/items/{id}",
		OperationID: "removeItem",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorDelete, result.Type)
	// L1 strong (0.8) + L3 boost (0.05) = 0.85
	assert.InDelta(t, 0.85, result.Confidence, 0.01)
}

func TestLayer3_NoSignal_DoesNotChangeResult(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/shipping",
		OperationID: "calculateShipping",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorCreate, result.Type,
		"no CRUD keyword match, L1 result (POST+collection=create) unchanged")
}

// --- Layer 3: skips HTTP method token ---

func TestLayer3_SkipsHTTPMethodToken(t *testing.T) {
	c := New()

	// "GetChargesCharge" — "get" matches the HTTP method (GET), should be
	// skipped so we don't produce a misleading fetch signal for a GET endpoint
	// (which is already fetch from L1 anyway — but we want clean semantics).
	op := parser.Operation{
		Method:      "GET",
		Path:        "/charges/{id}",
		OperationID: "GetChargesCharge",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type)
	// "Get" is skipped, no other keyword match → no L3 boost, stays at L1 confidence.
	assert.InDelta(t, 0.8, result.Confidence, 0.01)
}
