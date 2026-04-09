package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	// devVersion is the default version string for untagged builds.
	devVersion = "0.0.0-dev"

	// GitHub API and release URL constants.
	defaultReleaseURL  = "https://api.github.com/repos/mimikos-io/mimikos/releases/latest"
	releaseURLTemplate = "https://github.com/mimikos-io/mimikos/releases/tag/%s"
	goInstallCmd       = "go install github.com/mimikos-io/mimikos/cmd/mimikos@latest"
)

type (
	// updateResult holds the outcome of a version check.
	updateResult struct {
		CurrentVersion  string
		LatestVersion   string
		UpdateAvailable bool
	}

	// releaseResponse is the minimal GitHub Releases API response.
	releaseResponse struct {
		TagName string `json:"tag_name"` //nolint:tagliatelle // GitHub API uses snake_case
	}

	// semver holds the three numeric components of a semantic version.
	semver struct {
		major, minor, patch int
	}
)

// checkLatestVersion queries the GitHub Releases API and returns an
// [updateResult] when an update is available. Returns nil when the check
// should be silently skipped: dev builds, network failures, timeouts,
// non-200 responses, or when the current version is already up to date.
//
// The releaseURL parameter overrides the API endpoint for testing. Pass an
// empty string to use the default GitHub Releases URL.
func checkLatestVersion(ctx context.Context, currentVersion, releaseURL string) *updateResult {
	if currentVersion == devVersion {
		return nil
	}

	if releaseURL == "" {
		releaseURL = defaultReleaseURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil
	}

	if release.TagName == "" {
		return nil
	}

	if compareVersions(currentVersion, release.TagName) >= 0 {
		return nil // current version is equal or newer
	}

	return &updateResult{
		CurrentVersion:  currentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: true,
	}
}

// compareVersions compares two semantic version strings (major.minor.patch).
// Returns -1 if current < release, 0 if equal, 1 if current > release.
//
// Strips a leading "v" prefix before parsing. Returns 0 (safe default — no
// update shown) if either version cannot be parsed.
func compareVersions(current, release string) int {
	vCurrent, okCurrent := parseSemver(current)
	vRelease, okRelease := parseSemver(release)

	if !okCurrent || !okRelease {
		return 0
	}

	if vCurrent.major != vRelease.major {
		return cmpInt(vCurrent.major, vRelease.major)
	}

	if vCurrent.minor != vRelease.minor {
		return cmpInt(vCurrent.minor, vRelease.minor)
	}

	return cmpInt(vCurrent.patch, vRelease.patch)
}

// parseSemver parses a "major.minor.patch" string, stripping an optional "v"
// prefix. Returns the parsed version and true, or zero value and false on
// failure.
func parseSemver(s string) (semver, bool) {
	s = strings.TrimPrefix(s, "v")

	parts := strings.Split(s, ".")
	if len(parts) != 3 { //nolint:mnd // semantic versioning has exactly 3 parts
		return semver{}, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}

	return semver{major: major, minor: minor, patch: patch}, true
}

// cmpInt returns -1, 0, or 1 comparing two integers.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// formatUpdateNotification returns a human-friendly update notification
// showing the version transition and both installation methods (go install
// and pre-built binary download).
//
// Both version arguments are normalized to include the "v" prefix for
// display consistency with GitHub tags.
func formatUpdateNotification(current, latest string) string {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)

	return fmt.Sprintf(
		"\n⚡ Update available: %s → %s\n   %s\n   %s\n\n",
		current, latest,
		goInstallCmd,
		fmt.Sprintf(releaseURLTemplate, latest),
	)
}

// normalizeVersion ensures the version string has a "v" prefix for display
// consistency with GitHub tags.
func normalizeVersion(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}

	return "v" + v
}
