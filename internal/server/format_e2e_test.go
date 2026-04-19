package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Format Assertion E2E Tests (Task 32) ---

// buildFormatTestServer creates an httptest.Server from the format-test spec.
// Uses strict mode so response validation catches format violations.
func buildFormatTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	specBytes := loadSpecBytes(t, "e2e-format-test.yaml")

	handler, _, err := Build(context.Background(), specBytes, Config{Strict: true})
	require.NoError(t, err)

	return httptest.NewServer(handler)
}

func TestFormatE2E_DateTime(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// placed_at has format: date-time and no semantic mapper match.
	// Must produce an ISO 8601 timestamp, not a random string.
	placedAt, ok := event["placed_at"].(string)
	require.True(t, ok, "placed_at should be a string")

	_, err := time.Parse(time.RFC3339, placedAt)
	assert.NoError(t, err, "placed_at should be valid RFC 3339: got %q", placedAt)
}

func TestFormatE2E_Date(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// scheduled_for has format: date.
	scheduledFor, ok := event["scheduled_for"].(string)
	require.True(t, ok, "scheduled_for should be a string")

	_, err := time.Parse("2006-01-02", scheduledFor)
	assert.NoError(t, err, "scheduled_for should be valid date: got %q", scheduledFor)
}

func TestFormatE2E_UUID(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// correlation_id has format: uuid.
	correlationID, ok := event["correlation_id"].(string)
	require.True(t, ok, "correlation_id should be a string")

	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	assert.Regexp(t, uuidPattern, correlationID, "correlation_id should be a valid UUID: got %q", correlationID)
}

func TestFormatE2E_URI(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// notify_endpoint has format: uri.
	endpoint, ok := event["notify_endpoint"].(string)
	require.True(t, ok, "notify_endpoint should be a string")

	assert.Regexp(t, `^https?://`, endpoint, "notify_endpoint should be a URL: got %q", endpoint)
}

func TestFormatE2E_Hostname(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// origin has format: hostname.
	origin, ok := event["origin"].(string)
	require.True(t, ok, "origin should be a string")

	assert.NotEmpty(t, origin, "origin should not be empty")
	assert.Contains(t, origin, ".", "origin should look like a hostname: got %q", origin)
}

func TestFormatE2E_IPv4(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// gateway has format: ipv4.
	gateway, ok := event["gateway"].(string)
	require.True(t, ok, "gateway should be a string")

	ip := net.ParseIP(gateway)
	assert.NotNil(t, ip, "gateway should be a valid IP: got %q", gateway)
	assert.NotNil(t, ip.To4(), "gateway should be IPv4, not IPv6: got %q", gateway)
}

func TestFormatE2E_Email(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event := getJSONObject(t, srv, "/events/1")

	// contact has format: email and no semantic mapper match.
	contact, ok := event["contact"].(string)
	require.True(t, ok, "contact should be a string")

	assert.Contains(t, contact, "@", "contact should be an email address: got %q", contact)
}

func TestFormatE2E_Determinism(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	event1 := getJSONObject(t, srv, "/events/1")
	event2 := getJSONObject(t, srv, "/events/1")

	assert.Equal(t, event1, event2, "same request should produce identical format values")
}

func TestFormatE2E_StrictMode_PassesValidation(t *testing.T) {
	skipIfShort(t)

	// Server is built with Strict: true — format validation is enforced.
	// If generated format values are invalid, this would return 500.
	srv := buildFormatTestServer(t)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/events/1")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"strict mode: generated format values should pass format validation")
}

func TestFormatE2E_ListEvents_FormatValuesInArray(t *testing.T) {
	skipIfShort(t)

	srv := buildFormatTestServer(t)
	defer srv.Close()

	resp := doRequest(t, srv, http.MethodGet, "/events")
	defer closeBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var events []map[string]any
	decodeJSON(t, resp, &events)

	require.NotEmpty(t, events, "should return at least one event")

	// Verify format fields produce appropriate values in array items.
	for i, event := range events {
		placedAt, ok := event["placed_at"].(string)
		require.True(t, ok, "events[%d].placed_at should be a string", i)

		_, err := time.Parse(time.RFC3339, placedAt)
		assert.NoError(t, err, "events[%d].placed_at should be valid RFC 3339: got %q", i, placedAt)
	}
}
