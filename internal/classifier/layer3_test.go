package classifier

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
	"github.com/stretchr/testify/assert"
)

// --- Layer 3: operationId tokenization ---

func TestTokenize(t *testing.T) {
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
		{"space separated", "Update a customer", []string{"update", "a", "customer"}},
		{"empty", "", nil},
		{"single word", "list", []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Tokenize(tt.opID)
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

// --- Layer 3: "show" keyword ---

func TestLayer3_ShowKeyword_MapsFetch(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/account/{id}",
		OperationID: "showAccount",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type,
		"'show' keyword should map to fetch")
	assert.InDelta(t, 0.6, result.Confidence, 0.01,
		"L3 override confidence should be 0.6")
}

// --- Layer 3: summary scanning ---

func TestLayer3_Summary_UpdateKeyword_OverridesToUpdate(t *testing.T) {
	c := New()
	// Stripe POST-as-update: operationId has no CRUD keyword, summary does.
	op := parser.Operation{
		Method:      "POST",
		Path:        "/v1/customers/{customer}",
		OperationID: "PostCustomersCustomer",
		Summary:     "Update a customer",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorUpdate, result.Type,
		"summary 'Update a customer' should override POST-to-item to update")
	assert.InDelta(t, 0.6, result.Confidence, 0.01)
}

func TestLayer3_Summary_GetKeyword_MethodSkipped(t *testing.T) {
	c := New()
	// Summary starts with "Get" on a GET endpoint. "Get" is skipped (same
	// method-skip as operationId) because it's ambiguous — used for both
	// singletons ("Get Current User's Profile") and lists ("Get Followed
	// Artists"). Non-method verbs like "Retrieve"/"Show" are unambiguous.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/me",
		OperationID: "get-current-users-profile",
		Summary:     "Get Current User's Profile",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"summary 'Get' is method-skipped, no signal, stays list")
}

func TestLayer3_Summary_RetrieveKeyword_NotSkipped(t *testing.T) {
	c := New()
	// "Retrieve" is NOT the HTTP method, so it's not skipped in summary
	// scanning. However, summary signals cannot trigger the targeted
	// list→fetch override (too ambiguous for high-confidence overrides).
	// So this stays list despite the summary fetch signal.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/v1/account",
		OperationID: "GetAccount",
		Summary:     "Retrieve account",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"summary 'Retrieve' cannot trigger targeted override; stays list")
}

// --- Layer 3: targeted list<->fetch override ---

func TestLayer3_ListToFetch_SingletonOverride(t *testing.T) {
	c := New()

	// Only operationId signals at position > 0 (after a namespace prefix)
	// trigger the targeted list→fetch override. Position 0 keywords like
	// "FetchBalance" or "retrieveComments" are too ambiguous.
	tests := []struct {
		name        string
		path        string
		operationID string
		summary     string
	}{
		{"singleton /user via operationId get", "/user", "users/get-authenticated", "Get the authenticated user"},
		{"singleton /app via operationId", "/app", "apps/get-authenticated", "Get the authenticated app"},
		{"singleton /app/hook/config via operationId", "/app/hook/config",
			"apps/get-webhook-config-for-app", "Get a webhook configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "GET",
				Path:        tt.path,
				OperationID: tt.operationID,
				Summary:     tt.summary,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorFetch, result.Type,
				"list→fetch override should fire for singleton %s", tt.path)
		})
	}
}

func TestLayer3_FetchToList_AnalyticsNoOverrideFromSummary(t *testing.T) {
	c := New()

	// Analytics endpoints with item paths that return lists. Summary says
	// "List..." but summary signals cannot trigger the targeted override —
	// too ambiguous. These remain misclassified (accepted limitation).
	tests := []struct {
		name    string
		path    string
		summary string
	}{
		{"analytics videos", "/analytics/videos/{videoId}", "List video player sessions"},
		{"analytics live-streams", "/analytics/live-streams/{liveStreamId}", "List live stream player sessions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:  "GET",
				Path:    tt.path,
				Summary: tt.summary,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorFetch, result.Type,
				"summary 'List' cannot trigger targeted override; stays fetch")
		})
	}
}

func TestLayer3_FetchToList_OverrideFromOperationId(t *testing.T) {
	c := New()

	// OperationId signals CAN trigger fetch→list targeted override.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/analytics/videos/{videoId}",
		OperationID: "analytics/list-sessions",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"operationId 'list' signal should trigger fetch→list override")
}

func TestLayer3_ListToFetch_SafeSummaryKeywordOverride(t *testing.T) {
	c := New()

	// "show" and "fetch" in summary are unambiguous fetch markers that can
	// trigger the targeted list→fetch override.
	tests := []struct {
		name        string
		path        string
		operationID string
		summary     string
	}{
		{"show account (apivideo)", "/account", "GET_account", "Show account"},
		{"show video status (apivideo)", "/videos/{videoId}/status", "GET-video-status", "Show video status"},
		{"fetch balance (twilio)", "/2010-04-01/Accounts/{AccountSid}/Balance.json", "FetchBalance", "Fetch the balance"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "GET",
				Path:        tt.path,
				OperationID: tt.operationID,
				Summary:     tt.summary,
			}

			result := c.Classify(op)
			assert.Equal(t, model.BehaviorFetch, result.Type,
				"safe summary keyword should trigger list→fetch override")
		})
	}
}

func TestLayer3_ListToFetch_RetrieveNotSafe(t *testing.T) {
	c := New()

	// "retrieve" is NOT in the safe set — used for both singletons and lists.
	// Should NOT trigger the targeted override.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/v1/account",
		OperationID: "GetAccount",
		Summary:     "Retrieve account",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"'retrieve' is ambiguous — should not trigger targeted override")
}

func TestLayer3_ListToFetch_Position0NoOverride(t *testing.T) {
	c := New()

	// Keywords at position 0 in operationId are ambiguous — "retrieve" is
	// used for both single resources and collections. Should NOT trigger
	// the targeted override.
	tests := []struct {
		name        string
		path        string
		operationID string
		expected    model.BehaviorType
	}{
		{"retrieveComments stays list", "/v1/comments", "retrieveComments", model.BehaviorList},
		{"FetchBalance stays list", "/2010-04-01/Accounts/{AccountSid}/Balance.json", "FetchBalance", model.BehaviorList},
		{"find-pet-by-id stays fetch", "/pets/{id}", "find pet by id", model.BehaviorFetch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := parser.Operation{
				Method:      "GET",
				Path:        tt.path,
				OperationID: tt.operationID,
			}

			result := c.Classify(op)
			assert.Equal(t, tt.expected, result.Type,
				"position-0 keyword should not trigger targeted override")
		})
	}
}

func TestLayer3_ListToFetch_NoRegressionOnLegitimateList(t *testing.T) {
	c := New()

	// GET /users with no summary or operationId → should stay list.
	op := parser.Operation{
		Method: "GET",
		Path:   "/users",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type, "legitimate list should not be overridden")
}

func TestLayer3_ListToFetch_NoOverrideWithoutSignal(t *testing.T) {
	c := New()

	// GET / with operationId "meta/root" and summary "GitHub API Root" — no CRUD keyword.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/",
		OperationID: "meta/root",
		Summary:     "GitHub API Root",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"no CRUD keyword → no override, stays list")
}

func TestLayer3_Summary_ListKeyword(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/reports/{id}",
		OperationID: "PostReportsReport",
		Summary:     "List report entries",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"summary 'List report entries' should override POST-to-item to list")
	assert.InDelta(t, 0.6, result.Confidence, 0.01)
}

func TestLayer3_Summary_EmptyNoSignal(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/customers/{id}",
		OperationID: "PostCustomersCustomer",
		Summary:     "",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorGeneric, result.Type,
		"empty summary produces no signal")
}

func TestLayer3_Summary_OperationIdTakesPriority(t *testing.T) {
	c := New()
	// operationId says "update", summary says "delete".
	// operationId should win.
	op := parser.Operation{
		Method:      "POST",
		Path:        "/items/{id}",
		OperationID: "updateItem",
		Summary:     "Delete the item permanently",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorUpdate, result.Type,
		"operationId signal takes priority over summary")
}

func TestLayer3_Summary_ShowKeyword_MapsFetch(t *testing.T) {
	c := New()
	op := parser.Operation{
		Method:      "POST",
		Path:        "/account/{id}",
		OperationID: "PostAccountId",
		Summary:     "Show account details",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type,
		"summary 'Show account details' should map to fetch via 'show' keyword")
}

func TestLayer3_Summary_NounCollision_NoFalseSignal(t *testing.T) {
	c := New()

	// "Get Show Episodes" — "show" is a TV show (noun), not the verb "show".
	// Only the first token is checked, and it matches method → skipped.
	// "show" at position 1 is NOT checked.
	op := parser.Operation{
		Method:      "GET",
		Path:        "/shows/{id}/episodes",
		OperationID: "get-a-shows-episodes",
		Summary:     "Get Show Episodes",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorList, result.Type,
		"noun 'show' in summary should not trigger fetch signal")
}

func TestLayer3_Summary_BrowseNounCollision(t *testing.T) {
	c := New()

	// "Get a Browse Category" — "browse" is a section name (noun).
	op := parser.Operation{
		Method:      "GET",
		Path:        "/browse/categories/{id}",
		OperationID: "get-a-category",
		Summary:     "Get a Browse Category",
	}

	result := c.Classify(op)
	assert.Equal(t, model.BehaviorFetch, result.Type,
		"noun 'browse' in summary should not produce list signal")
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
