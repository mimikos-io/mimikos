package errors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- RFC 7807 Tests ---

func TestValidationError_SingleError(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.ValidationError(w, []model.ValidationError{
		{Field: "/body/name", Message: "property is required"},
	})

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusBadRequest, body.Status)
	assert.Equal(t, "Bad Request", body.Title)
	require.Len(t, body.Errors, 1)
	assert.Equal(t, "/body/name", body.Errors[0].Field)
	assert.Equal(t, "property is required", body.Errors[0].Message)
}

func TestValidationError_MultipleErrors(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	errs := []model.ValidationError{
		{Field: "/body/name", Message: "property is required"},
		{Field: "/body/age", Message: "expected integer, got string"},
		{Field: "query.limit", Message: "must be >= 1"},
	}
	r.ValidationError(w, errs)

	body := decodeProblemDetail(t, w)
	require.Len(t, body.Errors, 3)
	assert.Equal(t, "/body/name", body.Errors[0].Field)
	assert.Equal(t, "/body/age", body.Errors[1].Field)
	assert.Equal(t, "query.limit", body.Errors[2].Field)
}

func TestValidationError_ZeroErrors_OmitsErrorsKey(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.ValidationError(w, nil)

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusBadRequest, body.Status)
	assert.Nil(t, body.Errors)

	// Verify "errors" key is omitted from JSON (omitempty with nil slice).
	var raw map[string]any

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.NotContains(t, raw, "errors")
}

func TestNotFound(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.NotFound(w, http.MethodGet, "/pets/42")

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusNotFound, body.Status)
	assert.Equal(t, "Not Found", body.Title)
	assert.Contains(t, body.Detail, "GET")
	assert.Contains(t, body.Detail, "/pets/42")
}

func TestMethodNotAllowed(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.MethodNotAllowed(w, http.MethodDelete, "/pets", []string{"GET", "POST"})

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Equal(t, "GET, POST", resp.Header.Get("Allow"))

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusMethodNotAllowed, body.Status)
	assert.Equal(t, "Method Not Allowed", body.Title)
	assert.Contains(t, body.Detail, "DELETE")
	assert.Contains(t, body.Detail, "/pets")
}

func TestUnsupportedMediaType(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.UnsupportedMediaType(w, "text/xml")

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusUnsupportedMediaType, body.Status)
	assert.Equal(t, "Unsupported Media Type", body.Title)
	assert.Contains(t, body.Detail, "text/xml")
}

func TestNotAcceptable(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.NotAcceptable(w, "application/xml")

	resp := w.Result()
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotAcceptable, resp.StatusCode)

	body := decodeProblemDetail(t, w)
	assert.Equal(t, http.StatusNotAcceptable, body.Status)
	assert.Equal(t, "Not Acceptable", body.Title)
	assert.Contains(t, body.Detail, "application/xml")
}

func TestContentTypeProblemJSON(t *testing.T) {
	r := NewResponder()

	tests := []struct {
		name string
		call func(w http.ResponseWriter)
	}{
		{"ValidationError", func(w http.ResponseWriter) {
			r.ValidationError(w, []model.ValidationError{{Field: "f", Message: "m"}})
		}},
		{"NotFound", func(w http.ResponseWriter) {
			r.NotFound(w, http.MethodGet, "/x")
		}},
		{"MethodNotAllowed", func(w http.ResponseWriter) {
			r.MethodNotAllowed(w, http.MethodPut, "/x", []string{"GET"})
		}},
		{"UnsupportedMediaType", func(w http.ResponseWriter) {
			r.UnsupportedMediaType(w, "text/xml")
		}},
		{"NotAcceptable", func(w http.ResponseWriter) {
			r.NotAcceptable(w, "text/xml")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.call(w)

			resp := w.Result()
			defer resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			assert.Equal(t, contentTypeProblemJSON, ct) //nolint:testifylint // header string, not JSON
		})
	}
}

func TestTypeFieldAboutBlank(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.NotFound(w, http.MethodPost, "/x")

	body := decodeProblemDetail(t, w)
	assert.Equal(t, "about:blank", body.Type)
}

func TestBodyIsValidJSON(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.ValidationError(w, []model.ValidationError{
		{Field: "/body/x", Message: "bad"},
	})

	var raw map[string]any

	err := json.Unmarshal(w.Body.Bytes(), &raw)
	require.NoError(t, err)
}

// --- Truncation Tests ---

func TestNotFound_LongPathTruncated(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	longPath := "/" + strings.Repeat("a", 300)
	r.NotFound(w, http.MethodGet, longPath)

	body := decodeProblemDetail(t, w)
	assert.NotContains(t, body.Detail, longPath)
	assert.Contains(t, body.Detail, "...")
	assert.LessOrEqual(t, len(body.Detail), maxDetailLen+100) // prefix + truncated + "..."
}

func TestUnsupportedMediaType_LongContentTypeTruncated(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	longCT := "application/" + strings.Repeat("x", 300)
	r.UnsupportedMediaType(w, longCT)

	body := decodeProblemDetail(t, w)
	assert.NotContains(t, body.Detail, longCT)
	assert.Contains(t, body.Detail, "...")
}

func TestMethodNotAllowed_LongPathTruncated(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	longPath := "/" + strings.Repeat("b", 300)
	r.MethodNotAllowed(w, http.MethodPost, longPath, []string{"GET"})

	body := decodeProblemDetail(t, w)
	assert.NotContains(t, body.Detail, longPath)
	assert.Contains(t, body.Detail, "...")
}

// --- Edge Cases ---

func TestValidationError_EmptyFieldString(t *testing.T) {
	r := NewResponder()
	w := httptest.NewRecorder()

	r.ValidationError(w, []model.ValidationError{
		{Field: "", Message: "request body is required"},
	})

	body := decodeProblemDetail(t, w)
	require.Len(t, body.Errors, 1)
	assert.Empty(t, body.Errors[0].Field)
	assert.Equal(t, "request body is required", body.Errors[0].Message)
}

func TestTruncateDetail(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"short", "/pets/42", "/pets/42"},
		{"exact limit", strings.Repeat("a", maxDetailLen), strings.Repeat("a", maxDetailLen)},
		{"over limit", strings.Repeat("a", maxDetailLen+50), strings.Repeat("a", maxDetailLen) + "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, truncateDetail(tt.input))
		})
	}
}

// --- Test Helpers ---

// decodeProblemDetail unmarshals the response body into a problemDetail struct.
func decodeProblemDetail(t *testing.T, w *httptest.ResponseRecorder) problemDetail {
	t.Helper()

	var pd problemDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pd))

	return pd
}
