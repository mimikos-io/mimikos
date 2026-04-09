package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- checkLatestVersion tests ---

func TestCheckLatestVersion_SkipDev(t *testing.T) {
	t.Parallel()

	result := checkLatestVersion(context.Background(), "0.0.0-dev", "")
	assert.Nil(t, result, "dev builds should skip version check")
}

func TestCheckLatestVersion_UpdateAvailable(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.4.0"}`))
	}))
	defer srv.Close()

	result := checkLatestVersion(context.Background(), "0.3.1", srv.URL)

	require.NotNil(t, result)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "0.3.1", result.CurrentVersion)
	assert.Equal(t, "v0.4.0", result.LatestVersion)
}

func TestCheckLatestVersion_UpToDate(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.3.1"}`))
	}))
	defer srv.Close()

	result := checkLatestVersion(context.Background(), "0.3.1", srv.URL)
	assert.Nil(t, result, "up-to-date version should return nil")
}

func TestCheckLatestVersion_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := checkLatestVersion(ctx, "0.3.1", srv.URL)
	assert.Nil(t, result, "timeout should return nil")
}

func TestCheckLatestVersion_Non200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	result := checkLatestVersion(context.Background(), "0.3.1", srv.URL)
	assert.Nil(t, result, "non-200 response should return nil")
}

func TestCheckLatestVersion_BadJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	result := checkLatestVersion(context.Background(), "0.3.1", srv.URL)
	assert.Nil(t, result, "bad JSON should return nil")
}

func TestCheckLatestVersion_EmptyTagName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": ""}`))
	}))
	defer srv.Close()

	result := checkLatestVersion(context.Background(), "0.3.1", srv.URL)
	assert.Nil(t, result, "empty tag_name should return nil")
}

func TestCheckLatestVersion_RequestHeaders(t *testing.T) {
	t.Parallel()

	var gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.3.1"}`))
	}))
	defer srv.Close()

	_ = checkLatestVersion(context.Background(), "0.3.1", srv.URL)

	assert.Equal(t, "application/vnd.github+json", gotAccept, "should use GitHub API accept header")
}

// --- compareVersions tests ---

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want int
	}{
		// Equal versions.
		{name: "equal no prefix", a: "0.3.1", b: "0.3.1", want: 0},
		{name: "equal v prefix", a: "v0.3.1", b: "v0.3.1", want: 0},
		{name: "equal mixed prefix", a: "0.3.1", b: "v0.3.1", want: 0},

		// Less than.
		{name: "patch less", a: "0.3.0", b: "0.3.1", want: -1},
		{name: "minor less", a: "0.2.9", b: "0.3.0", want: -1},
		{name: "major less", a: "0.9.9", b: "1.0.0", want: -1},

		// Greater than.
		{name: "patch greater", a: "0.3.2", b: "0.3.1", want: 1},
		{name: "minor greater", a: "0.4.0", b: "0.3.9", want: 1},
		{name: "major greater", a: "2.0.0", b: "1.9.9", want: 1},

		// With v prefix on one or both sides.
		{name: "v prefix on a only", a: "v1.0.0", b: "0.9.0", want: 1},
		{name: "v prefix on b only", a: "0.9.0", b: "v1.0.0", want: -1},

		// Invalid versions return 0 (safe default: no update shown).
		{name: "invalid a", a: "not-a-version", b: "1.0.0", want: 0},
		{name: "invalid b", a: "1.0.0", b: "garbage", want: 0},
		{name: "both invalid", a: "abc", b: "def", want: 0},
		{name: "empty strings", a: "", b: "", want: 0},
		{name: "two parts only", a: "1.2", b: "1.2.3", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, compareVersions(tt.a, tt.b))
		})
	}
}

// --- formatUpdateNotification tests ---

func TestFormatUpdateNotification(t *testing.T) {
	t.Parallel()

	got := formatUpdateNotification("0.3.1", "v0.4.0")

	// Must contain the version transition.
	assert.Contains(t, got, "v0.3.1")
	assert.Contains(t, got, "v0.4.0")

	// Must contain both installation methods.
	assert.Contains(t, got, "go install github.com/mimikos-io/mimikos/cmd/mimikos@latest")
	assert.Contains(t, got, "https://github.com/mimikos-io/mimikos/releases/tag/v0.4.0")
}

func TestFormatUpdateNotification_NormalizesPrefix(t *testing.T) {
	t.Parallel()

	// Both inputs without "v" prefix — output should still show "v".
	got := formatUpdateNotification("1.0.0", "2.0.0")

	assert.Contains(t, got, "v1.0.0")
	assert.Contains(t, got, "v2.0.0")
}
