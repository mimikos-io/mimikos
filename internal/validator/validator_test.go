package validator

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadTestDoc loads a libopenapi.Document from the validation test spec.
//
//nolint:ireturn // libopenapi.NewDocument returns the Document interface
func loadTestDoc(t *testing.T) libopenapi.Document {
	t.Helper()

	data, err := os.ReadFile("../../testdata/specs/validation-test.yaml")
	require.NoError(t, err, "failed to read test spec")

	doc, err := libopenapi.NewDocument(data)
	require.NoError(t, err, "failed to parse test spec")

	return doc
}

// newValidator creates a LibopenAPIValidator from the test spec.
func newValidator(t *testing.T) *LibopenAPIValidator {
	t.Helper()

	v, err := NewLibopenAPIValidator(loadTestDoc(t))
	require.NoError(t, err, "failed to create validator")

	return v
}

// jsonRequest creates an *http.Request with JSON body for the given method and path.
func jsonRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}

	return req
}

// --- Constructor tests ---

func TestNewLibopenAPIValidator_ValidSpec(t *testing.T) {
	doc := loadTestDoc(t)

	v, err := NewLibopenAPIValidator(doc)

	require.NoError(t, err)
	assert.NotNil(t, v)
}

// --- Validation error tests ---

func TestValidate_MissingRequiredBodyField(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users", `{"age": 25, "status": "active"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for missing required field")

	// Missing required property is a root-level schema error — the field name
	// appears in the message, not in Field (which is empty for root-level errors).
	found := false

	for _, e := range errs {
		if strings.Contains(e.Message, "name") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error message mentioning 'name', got: %v", errs)
}

func TestValidate_TypeMismatch(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": "not-a-number", "status": "active"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for type mismatch")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "age") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'age' type mismatch, got: %v", errs)
}

func TestValidate_EnumViolation(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": 25, "status": "invalid"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for enum violation")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "status") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'status' enum violation, got: %v", errs)
}

func TestValidate_PatternMismatch(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": 25, "status": "active", "code": "bad"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for pattern mismatch")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "code") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'code' pattern mismatch, got: %v", errs)
}

func TestValidate_MinMaxViolation(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": -1, "status": "active"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for minimum violation")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "age") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'age' minimum violation, got: %v", errs)
}

func TestValidate_MissingRequiredQueryParam(t *testing.T) {
	v := newValidator(t)
	// GET /items without required 'limit' query param.
	req := jsonRequest(t, http.MethodGet, "/items", "")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for missing query param")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "limit") && strings.Contains(e.Field, "query") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for missing 'query.limit', got: %v", errs)
}

func TestValidate_MultipleErrorsCollected(t *testing.T) {
	v := newValidator(t)
	// Missing name (required), wrong type for age, invalid enum for status.
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"age": "not-a-number", "status": "invalid"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(errs), 3,
		"expected at least 3 validation errors, got %d: %v", len(errs), errs)
}

func TestValidate_ValidPostRequest(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": 25, "status": "active"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	assert.Empty(t, errs, "expected no validation errors for valid request, got: %v", errs)
}

func TestValidate_ValidGetRequest(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodGet, "/health", "")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	assert.Empty(t, errs, "expected no validation errors for valid GET, got: %v", errs)
}

func TestValidate_ValidGetWithQueryParams(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodGet, "/items?limit=10", "")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	assert.Empty(t, errs, "expected no validation errors for valid GET with params, got: %v", errs)
}

// --- Routing-level tests ---

func TestValidate_PathNotFound(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodGet, "/nonexistent", "")

	_, err := v.Validate(req)

	require.ErrorIs(t, err, ErrPathNotFound)
}

func TestValidate_OperationNotFound(t *testing.T) {
	v := newValidator(t)
	// DELETE is not defined for /users.
	req := jsonRequest(t, http.MethodDelete, "/users", "")

	_, err := v.Validate(req)

	require.ErrorIs(t, err, ErrOperationNotFound)
}

// --- Edge cases ---

func TestValidate_MissingRequiredBodyEntirely(t *testing.T) {
	v := newValidator(t)
	// POST /users with no body at all.
	req := jsonRequest(t, http.MethodPost, "/users", "")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for missing body")

	// Should have a request-level error (empty or general field).
	found := false

	for _, e := range errs {
		if e.Field == "" || strings.Contains(strings.ToLower(e.Message), "body") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected request-level error for missing body, got: %v", errs)
}

func TestValidate_WrongContentType(t *testing.T) {
	v := newValidator(t)
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(`{"name": "Alice"}`))
	req.Header.Set("Content-Type", "text/plain")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for wrong content type")
}

func TestValidate_FormatViolation(t *testing.T) {
	v := newValidator(t)
	req := jsonRequest(t, http.MethodPost, "/users",
		`{"name": "Alice", "age": 25, "status": "active", "email": "not-an-email"}`)

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for format violation")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "email") || strings.Contains(e.Message, "email") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'email' format violation, got: %v", errs)
}

func TestValidate_PathParamTypeMismatch(t *testing.T) {
	v := newValidator(t)
	// userId expects integer, send a string.
	req := jsonRequest(t, http.MethodGet, "/users/abc", "")

	errs, err := v.Validate(req)

	require.NoError(t, err)
	require.NotEmpty(t, errs, "expected validation errors for path param type mismatch")

	found := false

	for _, e := range errs {
		if strings.Contains(e.Field, "path") && strings.Contains(e.Field, "userId") {
			found = true

			break
		}
	}

	assert.True(t, found, "expected error for 'path.userId' type mismatch, got: %v", errs)
}

// --- Unit tests for fieldPathToJSONPointer ---

func TestFieldPathToJSONPointer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "bare root $", input: "$", want: ""},
		{name: "single field", input: "$.name", want: "/name"},
		{name: "nested field", input: "$.address.city", want: "/address/city"},
		{name: "deeply nested", input: "$.a.b.c.d", want: "/a/b/c/d"},
		{name: "no dollar prefix", input: "name", want: "/name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldPathToJSONPointer(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
