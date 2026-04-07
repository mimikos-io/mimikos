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

	// segments holds the normalized, non-empty path segments. Used by
	// sub-resource detection to check for parent path parameters.
	segments []string
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

	// Sub-resource delete: DELETE /resource/{id}/sub-resource.
	// The parent path parameter scopes the sub-resource, and the singular
	// last segment names the specific sub-resource being deleted (e.g.,
	// DELETE /live-streams/{liveStreamId}/thumbnail).
	//
	// Excludes:
	//   - /me/albums: no parent path parameter
	//   - /playlists/{id}/tracks: plural "tracks" → bulk collection operation
	//   - /installations/{id}/suspended: past participle → state toggle
	if hasParentPathParam(info.segments) && looksLikeSingularSubResource(info.lastSegment) {
		return Result{Type: model.BehaviorDelete, Confidence: confidenceModerate}
	}

	// DELETE on collection without parent param: bulk delete (e.g., DELETE /me/albums).
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
		segments:    segments,
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
	tokens := Tokenize(segment)
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

// looksLikeSingularSubResource returns true if the segment name looks like a
// singular sub-resource that can be individually deleted. Returns false for:
//   - plural names ending in 's' (collection/bulk operations: tracks, followers)
//   - past participles ending in 'ed' (state toggles: suspended, enabled)
func looksLikeSingularSubResource(segment string) bool {
	lower := strings.ToLower(segment)

	if strings.HasSuffix(lower, "s") {
		return false
	}

	if strings.HasSuffix(lower, "ed") {
		return false
	}

	return true
}

// hasParentPathParam returns true if any segment before the last one is a path
// parameter. This indicates the final static segment is a sub-resource owned
// by the parametrized parent (e.g., /live-streams/{liveStreamId}/thumbnail).
// Returns false for paths like /me/albums where no segment is a parameter.
// minSubResourceSegments is the minimum path depth for a sub-resource pattern.
// A sub-resource like /resource/{id}/sub requires at least 3 segments, but the
// parent param check only examines segments[:len-1], so we need at least 2.
const minSubResourceSegments = 2

func hasParentPathParam(segments []string) bool {
	if len(segments) < minSubResourceSegments {
		return false
	}

	// Check all segments except the last (which is the sub-resource itself).
	for _, seg := range segments[:len(segments)-1] {
		if isPathParam(seg) {
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
