package state

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// versionSegment matches path segments like "v1", "v2", "api".
var versionSegment = regexp.MustCompile(`^(?:v\d+|api)$`)

// InferResourceIdentity extracts a resource type and ID from the path pattern,
// path parameters, response body, and an optional ID field hint. It uses a
// six-strategy approach:
//
//  1. Exact match: path param name matches body field (case-insensitive)
//  2. Suffix strip: strip resource prefix from param, match remainder in body
//  3. ID field hint: precomputed from fetch variant's path param
//  4. Body "id" fallback: universal convention
//  5. Path param value: from pathParams map (for fetch/update/delete where body is nil)
//  6. Deterministic UUID fallback
//
// Resource type is the collection segment preceding the last path parameter,
// or the last non-version segment for parameterless paths.
func InferResourceIdentity(
	pathPattern string,
	pathParams map[string]string,
	responseBody any,
	idFieldHint string,
) (string, string) {
	resourceType := extractResourceType(pathPattern)
	resourceID := extractResourceID(pathPattern, pathParams, responseBody, idFieldHint)

	return resourceType, resourceID
}

// ResourceType returns the collection name from a URL path pattern.
// This is the resource type key used for state storage.
//
// For "/users/{userId}/orders/{orderId}", it returns "orders".
// For "/users", it returns "users".
// Version segments (v1, v2, api) are skipped.
func ResourceType(pathPattern string) string {
	return extractResourceType(pathPattern)
}

// extractResourceType returns the collection name from a path pattern.
// For "/users/{userId}/orders/{orderId}", it returns "orders".
// For "/users", it returns "users".
// Version segments (v1, v2, api) are skipped.
func extractResourceType(pathPattern string) string {
	segments := splitPath(pathPattern)

	// Walk backwards to find the last non-param, non-version segment.
	// For item paths like /pets/{petId}, the collection is "pets" (before the param).
	// For collection paths like /pets, the collection is "pets" (the last segment).
	var lastCollection string

	for _, seg := range segments {
		if isParam(seg) {
			continue
		}

		if versionSegment.MatchString(seg) {
			continue
		}

		lastCollection = seg
	}

	return lastCollection
}

// extractResourceID extracts a resource ID using the six-strategy approach.
// Strategies 1-4 examine the response body (used by handleCreate which has
// a generated response). Strategy 5 uses pathParams (used by fetch/update/delete
// where body is nil). Strategy 6 is the deterministic UUID fallback.
//
//nolint:gocognit // sequential strategy chain — splitting would obscure the priority order
func extractResourceID(
	pathPattern string,
	pathParams map[string]string,
	responseBody any,
	idFieldHint string,
) string {
	body, ok := responseBody.(map[string]any)
	lastParam := LastPathParam(pathPattern)

	// Strategy 1: Exact match — path param name matches body field (case-insensitive).
	// Covers: Notion (id), Spotify (id), api.video (liveStreamId), Twilio (Sid→sid).
	if ok && lastParam != "" {
		if val := findField(body, lastParam); val != nil {
			return coerceToString(val)
		}
	}

	// Strategy 2: Suffix strip — strip resource prefix from param name, match remainder.
	// Covers: Petstore (petId→id), Asana (task_gid→gid), GitHub (comment_id→id).
	if ok && lastParam != "" {
		resourceType := extractResourceType(pathPattern)
		if remainder := StripResourcePrefix(lastParam, resourceType); remainder != "" {
			if val := findField(body, remainder); val != nil {
				return coerceToString(val)
			}
		}
	}

	// Strategy 3: ID field hint — precomputed from fetch variant's path param.
	// Covers: create operations with no path param (POST /projects → gid).
	// Only set when suffix-stripping produces a result (see computeIDFieldHint).
	if ok && idFieldHint != "" {
		if val := findField(body, idFieldHint); val != nil {
			return coerceToString(val)
		}
	}

	// Strategy 4: Body "id" fallback — universal convention.
	// Covers: Stripe ({customer}→id), any spec using plain "id".
	if ok {
		if val := findField(body, "id"); val != nil {
			return coerceToString(val)
		}
	}

	// Strategy 5: Path param value from params map (for fetch/update/delete).
	// When body is nil (non-create operations), the ID comes from the URL path.
	if lastParam != "" {
		if val, exists := pathParams[lastParam]; exists {
			return val
		}
	}

	// Strategy 6: Deterministic fallback UUID.
	return deterministicUUID(pathPattern, responseBody)
}

// findField looks up a field by name, exact match first, then case-insensitive.
// Returns nil if not found.
func findField(body map[string]any, name string) any {
	if val, exists := body[name]; exists {
		return val
	}

	for key, val := range body {
		if strings.EqualFold(key, name) {
			return val
		}
	}

	return nil
}

// LastPathParam returns the name of the last path parameter in a pattern.
// For "/users/{userId}/orders/{orderId}", it returns "orderId".
// Returns empty string if no parameters are present.
func LastPathParam(pathPattern string) string {
	segments := splitPath(pathPattern)

	for i := len(segments) - 1; i >= 0; i-- {
		if isParam(segments[i]) {
			return segments[i][1 : len(segments[i])-1] // strip { }
		}
	}

	return ""
}

// StripResourcePrefix strips the singular resource type prefix from a path
// parameter name. Handles both camelCase and underscore separators.
//
// Examples:
//   - StripResourcePrefix("petId", "pets") → "id" (camelCase: pet + Id)
//   - StripResourcePrefix("task_gid", "tasks") → "gid" (underscore: task_ + gid)
//   - StripResourcePrefix("comment_id", "comments") → "id" (underscore: comment_ + id)
//   - StripResourcePrefix("id", "pets") → "" (no prefix to strip)
//   - StripResourcePrefix("customer", "customers") → "" (no remainder after strip)
func StripResourcePrefix(paramName, resourceType string) string {
	singular := Singularize(resourceType)
	if singular == "" {
		return ""
	}

	lowerParam := strings.ToLower(paramName)
	lowerSingular := strings.ToLower(singular)

	// Check underscore separator: "task_gid" → strip "task_" → "gid"
	if strings.HasPrefix(lowerParam, lowerSingular+"_") {
		remainder := paramName[len(singular)+1:] // preserve original case
		if remainder != "" {
			return remainder
		}
	}

	// Check camelCase boundary: "petId" → strip "pet" → "Id" → lowercase → "id"
	if strings.HasPrefix(lowerParam, lowerSingular) && len(paramName) > len(singular) {
		rest := paramName[len(singular):]
		// Verify camelCase boundary: next char must be uppercase.
		if len(rest) > 0 && rest[0] >= 'A' && rest[0] <= 'Z' {
			return strings.ToLower(rest[:1]) + rest[1:]
		}
	}

	return ""
}

// Singularize performs naive English singularization of a collection name.
// Handles common URL path segment patterns: "pets"→"pet", "tasks"→"task",
// "categories"→"category", "addresses"→"address", "statuses"→"status".
//
// Known limitations: irregular plurals (indices→index, people→person) are not
// handled. These are extremely rare as URL path segments.
func Singularize(plural string) string {
	if plural == "" {
		return ""
	}

	lower := strings.ToLower(plural)

	// -ies → -y: "categories" → "category"
	if strings.HasSuffix(lower, "ies") {
		return plural[:len(plural)-3] + "y"
	}

	// -ses, -xes, -zes, -ches, -shes → drop "es": "addresses" → "address"
	if strings.HasSuffix(lower, "ses") || strings.HasSuffix(lower, "xes") ||
		strings.HasSuffix(lower, "zes") || strings.HasSuffix(lower, "ches") ||
		strings.HasSuffix(lower, "shes") {
		return plural[:len(plural)-2]
	}

	// -s → drop "s": "pets" → "pet"
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") {
		return plural[:len(plural)-1]
	}

	// Already singular or uncountable.
	return plural
}

// splitPath splits a URL path into non-empty segments.
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))

	for _, p := range parts {
		if p != "" {
			segments = append(segments, p)
		}
	}

	return segments
}

// isParam returns true if the segment is a path parameter (e.g., "{petId}").
func isParam(segment string) bool {
	return len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}'
}

// coerceToString converts a value to its string representation.
// Handles string, float64 (JSON numbers), json.Number, and other types.
func coerceToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		// Avoid trailing .0 for integer values.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}

		return strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// deterministicUUID generates a UUID v4-formatted string deterministically
// from the path pattern and response body. Same inputs always produce the
// same UUID.
func deterministicUUID(pathPattern string, body any) string {
	h := sha256.New()
	h.Write([]byte(pathPattern))
	h.Write([]byte{0})
	_, _ = fmt.Fprintf(h, "%v", body)

	sum := h.Sum(nil)

	// Format as UUID v4: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// Set version (4) and variant (RFC 4122) bits.
	const (
		uuidVersion4       = 0x40
		uuidVariantRFC4122 = 0x80
		versionMask        = 0x0f
		variantMask        = 0x3f
	)

	sum[6] = (sum[6] & versionMask) | uuidVersion4
	sum[8] = (sum[8] & variantMask) | uuidVariantRFC4122

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(sum[0:4]),
		binary.BigEndian.Uint16(sum[4:6]),
		binary.BigEndian.Uint16(sum[6:8]),
		binary.BigEndian.Uint16(sum[8:10]),
		sum[10:16],
	)
}
