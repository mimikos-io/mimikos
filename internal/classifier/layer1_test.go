package classifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Path Normalization ---

func TestNormalizePath_StripsJSONExtension(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "twilio style accounts",
			path: "/2010-04-01/Accounts.json",
			want: "/2010-04-01/Accounts",
		},
		{
			name: "twilio style with param",
			path: "/2010-04-01/Accounts/{AccountSid}.json",
			want: "/2010-04-01/Accounts/{AccountSid}",
		},
		{
			name: "xml extension",
			path: "/api/data.xml",
			want: "/api/data",
		},
		{
			name: "no extension unchanged",
			path: "/pets/{petId}",
			want: "/pets/{petId}",
		},
		{
			name: "json in middle segment unchanged",
			path: "/api/json/items",
			want: "/api/json/items",
		},
		{
			name: "empty path",
			path: "",
			want: "",
		},
		{
			name: "root path",
			path: "/",
			want: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePath(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Path Type Classification ---

func TestAnalyzePath_ItemPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"simple item", "/pets/{petId}"},
		{"nested item", "/users/{userId}/orders/{orderId}"},
		{"numeric style param", "/items/{id}"},
		{"twilio after normalization", "/2010-04-01/Accounts/{AccountSid}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzePath(tt.path)
			assert.True(t, info.isItem, "expected item path for %s", tt.path)
		})
	}
}

func TestAnalyzePath_CollectionPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"simple collection", "/pets"},
		{"nested collection", "/users/{userId}/orders"},
		{"browse subcollection", "/browse/categories"},
		{"deep nested", "/users/{userId}/accounts/{accountId}/transactions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzePath(tt.path)
			assert.False(t, info.isItem, "expected collection (not item) for %s", tt.path)
		})
	}
}

func TestAnalyzePath_ActionVerbs(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		isAction bool
	}{
		{"capture on item", "/charges/{id}/capture", true},
		{"cancel on item", "/payment_intents/{id}/cancel", true},
		{"confirm on item", "/payment_intents/{id}/confirm", true},
		{"merge on item", "/pulls/{number}/merge", true},
		{"pause on singleton", "/me/player/pause", true},
		{"play on singleton", "/me/player/play", true},
		{"search at root", "/search", true},
		{"query on item", "/databases/{id}/query", true},
		{"contains check", "/me/albums/contains", true},
		{"verify action", "/email/verify", true},
		{"send action", "/messages/send", true},
		{"not action - pets collection", "/pets", false},
		{"not action - orders collection", "/users/{id}/orders", false},
		{"not action - tracks resource", "/tracks", false},
		{"not action - categories resource", "/browse/categories", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzePath(tt.path)
			assert.Equal(t, tt.isAction, info.isAction,
				"expected isAction=%v for %s", tt.isAction, tt.path)
		})
	}
}

func TestAnalyzePath_LastSegment(t *testing.T) {
	tests := []struct {
		path        string
		lastSegment string
	}{
		{"/pets", "pets"},
		{"/pets/{petId}", "{petId}"},
		{"/users/{id}/orders", "orders"},
		{"/charges/{id}/capture", "capture"},
		{"/", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			info := analyzePath(tt.path)
			assert.Equal(t, tt.lastSegment, info.lastSegment)
		})
	}
}

func TestAnalyzePath_EmptyAndRoot(t *testing.T) {
	root := analyzePath("/")
	assert.False(t, root.isItem, "root path is collection (not item)")
	assert.False(t, root.isAction)

	empty := analyzePath("")
	assert.False(t, empty.isItem, "empty path defaults to collection (not item)")
	assert.False(t, empty.isAction)
}

// TestAnalyzePath_PluralResourceNotAction verifies that plural resource names
// that share a root with action verbs (e.g., "volumes" vs "volume") are NOT
// flagged as actions. The action verb map uses exact singular matches.
func TestAnalyzePath_PluralResourceNotAction(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"volumes collection", "/volumes"},
		{"volumes item", "/volumes/{id}"},
		{"searches collection", "/searches"},
		{"repeats collection", "/repeats"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzePath(tt.path)
			assert.False(t, info.isAction,
				"plural resource %q should not be flagged as action", tt.path)
		})
	}
}

// TestAnalyzePath_SingularActionVerbOnItemPath documents that a singular
// resource name matching an action verb WILL be flagged as action when it
// appears as the penultimate segment (e.g., PATCH /volume/{id}). This is
// a known Layer 1 limitation — Layer 2/3 can override via schema/operationId.
func TestAnalyzePath_SingularActionVerbOnItemPath(t *testing.T) {
	info := analyzePath("/volume/{id}")
	assert.True(t, info.isAction,
		"singular 'volume' on item path is flagged as action (known L1 limitation)")
}
