package classifier

import "strings"

// knownExtensions are file extensions stripped from the last path segment
// during normalization. Twilio uses .json suffixes on all paths.
//
//nolint:gochecknoglobals // immutable lookup set
var knownExtensions = []string{".json", ".xml"}

// actionVerbs are HTTP action verbs that indicate a non-CRUD operation when
// found as the last non-parameter segment of a path. Derived from corpus
// analysis of real-world APIs (Stripe, Spotify, GitHub, Asana, SendGrid, Notion).
//
// Uses exact singular match — plural resource names (e.g., "volumes", "searches")
// will NOT match. This means a singular resource that collides with an action verb
// (e.g., PATCH /volume/{id}) would be misclassified as action; Layers 2/3 resolve this.
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
		_, info.isAction = actionVerbs[strings.ToLower(actionSegment)]
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

// isPathParam returns true if the segment is an OpenAPI path parameter (e.g., "{petId}").
// Minimum length 3 covers the shortest valid param "{x}" — rejects empty "{}" or lone "{".
func isPathParam(segment string) bool {
	return len(segment) >= 3 && segment[0] == '{' && segment[len(segment)-1] == '}'
}
