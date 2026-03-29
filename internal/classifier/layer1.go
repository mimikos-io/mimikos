// Layer 1: HTTP method + path pattern classification.
//
// Layer 1 is the first and strongest signal in the classifier pipeline. It
// classifies operations based solely on the HTTP method and URL path structure:
//
//   - Path type: collection (/pets) vs item (/pets/{petId})
//   - Action verbs: known verbs in the last path segment (/charges/{id}/capture)
//   - Method dispatch: GET→list/fetch, POST→create, PUT/PATCH→update, DELETE→delete
//
// Layer 1 achieves ~85% accuracy on real-world API corpora. Singletons
// (GET /me, /account) and POST-to-item ambiguity are known limitations
// resolved by Layers 2 and 3.

package classifier

import (
	"net/http"
	"strings"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// knownExtensions are file extensions stripped from the last path segment
// during normalization. Twilio uses .json suffixes on all paths.
//
//nolint:gochecknoglobals // immutable lookup set
var knownExtensions = []string{".json", ".xml"}

// actionVerbs are HTTP action verbs that indicate a non-CRUD operation when
// found in the last non-parameter segment of a path. Derived from corpus
// analysis of real-world APIs (Stripe, Spotify, GitHub, Asana, SendGrid, Notion,
// API.video).
//
// Checked against both exact segment match and individual tokens when the
// segment is camelCase (e.g., "addFollowers" → tokens ["add", "followers"] →
// "add" matches). Plural forms (e.g., "volumes", "searches") do NOT match
// on exact check, but may match on tokenized check if the stem is an action verb.
//
// A singular resource name that collides with an action verb
// (e.g., PATCH /volume/{id}) would be misclassified as action. This is
// a known Layer 1 limitation — Layers 2/3 resolve via schema/operationId.
//
//nolint:gochecknoglobals // immutable lookup set
var actionVerbs = map[string]struct{}{
	// Transaction / lifecycle actions
	"capture":   {},
	"cancel":    {},
	"confirm":   {},
	"approve":   {},
	"reject":    {},
	"suspend":   {},
	"unsuspend": {},
	"convert":   {},
	"duplicate": {},
	"merge":     {},
	"retry":     {},

	// Media / playback control
	"pause":    {},
	"play":     {},
	"next":     {},
	"previous": {},
	"seek":     {},
	"shuffle":  {},
	"repeat":   {},
	"volume":   {},

	// Communication actions
	"send":   {},
	"resend": {},

	// Query / search / check actions
	"query":    {},
	"search":   {},
	"contains": {},
	"verify":   {},
	"validate": {},

	// Collection modification actions (Asana: addFollowers, removeItem)
	"add":    {},
	"remove": {},

	// Utility / process actions
	"refresh": {},
	"batch":   {},
	"insert":  {},
}

// pathInfo holds the results of analyzing a URL path.
type pathInfo struct {
	// isItem is true when the last path segment is a parameter (e.g., {petId}).
	// When false, the path is a collection (no param suffix).
	isItem bool

	// isAction is true when the last non-parameter segment is a known action verb.
	isAction bool

	// lastSegment is the final segment of the normalized path (without leading slash).
	lastSegment string
}

// applyLayer1 classifies based on HTTP method and path structure.
func (c *Classifier) applyLayer1(op parser.Operation) Result {
	info := analyzePath(op.Path)

	// Action verbs always produce generic, regardless of method.
	if info.isAction {
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
	}

	switch op.Method {
	case http.MethodGet:
		return c.classifyGET(info)
	case http.MethodPost:
		return c.classifyPOST(info)
	case http.MethodPut:
		return c.classifyPUT(info)
	case http.MethodPatch:
		return c.classifyPATCH(info)
	case http.MethodDelete:
		return c.classifyDELETE(info)
	default:
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceWeak}
	}
}

func (c *Classifier) classifyGET(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorFetch, Confidence: confidenceStrong}
	}

	return Result{Type: model.BehaviorList, Confidence: confidenceStrong}
}

func (c *Classifier) classifyPOST(info pathInfo) Result {
	if info.isItem {
		// POST to item path: could be update (Stripe/Twilio) or action (GitHub).
		// Layer 1 cannot distinguish — classify as generic, defer to L2/L3.
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceWeak}
	}

	return Result{Type: model.BehaviorCreate, Confidence: confidenceStrong}
}

func (c *Classifier) classifyPUT(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorUpdate, Confidence: confidenceStrong}
	}

	// PUT to collection: bulk operation (e.g., PUT /me/albums on Spotify).
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}

func (c *Classifier) classifyPATCH(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorUpdate, Confidence: confidenceStrong}
	}

	// PATCH to collection: unusual, treat as generic.
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}

func (c *Classifier) classifyDELETE(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorDelete, Confidence: confidenceStrong}
	}

	// DELETE on collection: bulk delete (e.g., DELETE /me/albums on Spotify).
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}

// normalizePath strips known file extensions from the last segment of a URL path.
func normalizePath(path string) string {
	if path == "" || path == "/" {
		return path
	}

	for _, ext := range knownExtensions {
		if strings.HasSuffix(path, ext) {
			return path[:len(path)-len(ext)]
		}
	}

	return path
}

// analyzePath normalizes a URL path and determines its structural type.
func analyzePath(raw string) pathInfo {
	path := normalizePath(raw)

	segments := splitSegments(path)
	if len(segments) == 0 {
		return pathInfo{}
	}

	last := segments[len(segments)-1]

	info := pathInfo{
		lastSegment: last,
		isItem:      isPathParam(last),
	}

	// Check for action verbs: the last non-parameter segment.
	actionSegment := last
	if info.isItem && len(segments) >= 2 {
		actionSegment = segments[len(segments)-2]
	}

	if !isPathParam(actionSegment) {
		info.isAction = isActionVerb(actionSegment)
	}

	return info
}

// splitSegments splits a URL path into non-empty segments.
func splitSegments(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))

	for _, p := range parts {
		if p != "" {
			segments = append(segments, p)
		}
	}

	return segments
}

// isActionVerb checks whether a path segment contains an action verb.
// First checks exact match on the lowercased segment (e.g., "capture").
// If no exact match, tokenizes camelCase/hyphenated segments and checks
// each token (e.g., "addFollowers" → ["add", "followers"] → "add" matches).
func isActionVerb(segment string) bool {
	lower := strings.ToLower(segment)

	// Exact match first (fast path).
	if _, ok := actionVerbs[lower]; ok {
		return true
	}

	// Tokenize camelCase/hyphenated segments for compound action names.
	tokens := TokenizeOperationID(segment)
	if len(tokens) <= 1 {
		// Single token already checked above.
		return false
	}

	for _, token := range tokens {
		if _, ok := actionVerbs[token]; ok {
			return true
		}
	}

	return false
}

// isPathParam returns true if the segment is an OpenAPI path parameter (e.g., "{petId}").
// Minimum length 3 covers the shortest valid param "{x}" — rejects empty "{}" or lone "{".
func isPathParam(segment string) bool {
	return len(segment) >= 3 && segment[0] == '{' && segment[len(segment)-1] == '}'
}
