package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"sort"
)

// Fingerprint computes a deterministic int64 seed from an HTTP request's
// identity. The result is canonical: query parameter order and JSON body
// key order do not affect the output.
//
// The fingerprint includes method, path, sorted query parameters, and
// canonicalized body. Identical requests always produce identical seeds
// across process restarts.
//
// The path parameter is the concrete request path (e.g., "/users/123"),
// not the OpenAPI template (e.g., "/users/{userId}"). Different path
// parameter values produce different seeds — this is the primary mechanism
// for varying mock responses across resource instances.
//
// Nil or empty query and body are treated as absent and produce the same
// fingerprint as each other.
func Fingerprint(method, path string, query url.Values, body []byte) int64 {
	canonical := method + "\x00" + path + "\x00" + canonicalQuery(query) + "\x00" + canonicalBody(body)
	h := sha256.Sum256([]byte(canonical))

	return int64(binary.BigEndian.Uint64(h[:8])) //nolint:gosec // intentional truncation of hash to int64
}

// canonicalQuery produces a deterministic string from URL query parameters.
// Keys are sorted lexicographically, and values for each key are also sorted.
func canonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var result []byte

	for i, k := range keys {
		vals := make([]string, len(query[k]))
		copy(vals, query[k])
		sort.Strings(vals)

		for j, v := range vals {
			if i > 0 || j > 0 {
				result = append(result, '&')
			}

			result = append(result, k...)
			result = append(result, '=')
			result = append(result, v...)
		}
	}

	return string(result)
}

// canonicalBody produces a deterministic string from a request body.
// If the body is valid JSON, it is re-serialized with sorted keys using
// json.Number to preserve full numeric precision (integers beyond float64
// range are not rounded). Otherwise, the raw bytes are used as-is.
// Nil or empty body returns an empty string.
func canonicalBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var parsed any
	if dec.Decode(&parsed) == nil {
		if canonical, err := json.Marshal(parsed); err == nil {
			return string(canonical)
		}
	}

	return string(body)
}
