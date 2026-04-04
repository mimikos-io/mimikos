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
// path parameters, and response body. It uses a three-strategy approach:
//
//  1. Top-level "id" field in the response body (case-insensitive)
//  2. Last path parameter name matched against path params map
//  3. Fallback: deterministic UUID derived from path and body
//
// Resource type is the collection segment preceding the last path parameter,
// or the last non-version segment for parameterless paths.
func InferResourceIdentity(
	pathPattern string,
	pathParams map[string]string,
	responseBody any,
) (string, string) {
	resourceType := extractResourceType(pathPattern)
	resourceID := extractResourceID(pathPattern, pathParams, responseBody)

	return resourceType, resourceID
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

// extractResourceID extracts a resource ID using the three-strategy approach.
func extractResourceID(
	pathPattern string,
	pathParams map[string]string,
	responseBody any,
) string {
	body, ok := responseBody.(map[string]any)

	// Strategy 1: top-level "id" field — exact match first, then case-insensitive
	// fallback. Exact-first eliminates non-determinism from Go's randomized map
	// iteration when multiple case variants exist (e.g., "id" and "ID").
	if ok {
		if val, exists := body["id"]; exists {
			return coerceToString(val)
		}

		for key, val := range body {
			if strings.EqualFold(key, "id") {
				return coerceToString(val)
			}
		}
	}

	// Strategy 2: last path parameter value from params map.
	lastParam := lastPathParam(pathPattern)
	if lastParam != "" {
		if val, exists := pathParams[lastParam]; exists {
			return val
		}

		// Also check if the param name matches a body field.
		if ok {
			if val, exists := body[lastParam]; exists {
				return coerceToString(val)
			}
		}
	}

	// Strategy 3: deterministic fallback UUID.
	return deterministicUUID(pathPattern, responseBody)
}

// lastPathParam returns the name of the last path parameter in a pattern.
// For "/users/{userId}/orders/{orderId}", it returns "orderId".
// Returns empty string if no parameters are present.
func lastPathParam(pathPattern string) string {
	segments := splitPath(pathPattern)

	for i := len(segments) - 1; i >= 0; i-- {
		if isParam(segments[i]) {
			return segments[i][1 : len(segments[i])-1] // strip { }
		}
	}

	return ""
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
