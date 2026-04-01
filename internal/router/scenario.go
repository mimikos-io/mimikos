package router

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mimikos-io/mimikos/internal/model"
)

// statusHeader is the HTTP header for explicit status code selection.
const statusHeader = "X-Mimikos-Status"

// ErrInvalidStatusCode is returned when the X-Mimikos-Status header value
// is not a valid integer.
var ErrInvalidStatusCode = errors.New("invalid status code")

// ErrStatusNotAvailable is returned when the requested status code is not
// defined in the operation's response schemas.
var ErrStatusNotAvailable = errors.New("status code not available for this operation")

// SelectedScenario is the output of scenario selection: which status code
// to serve and which schema to generate from.
type SelectedScenario struct {
	// StatusCode is the HTTP status code to return.
	StatusCode int

	// Schema is the compiled response schema to generate data from, or nil
	// if the operation defines no response schema for this status code.
	Schema *model.CompiledSchema
}

// SelectScenario picks the response scenario for a matched operation.
// When requestedStatus is empty, it returns the success scenario (backward-
// compatible default). When a status code is explicitly requested via the
// X-Mimikos-Status header, it validates against the entry's available
// response codes and returns the matching schema.
func SelectScenario(entry *model.BehaviorEntry, requestedStatus string) (*SelectedScenario, error) {
	if requestedStatus == "" {
		return selectSuccess(entry), nil
	}

	code, err := strconv.Atoi(requestedStatus)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStatusCode, requestedStatus)
	}

	// Accept the success code explicitly too.
	if code == entry.SuccessCode {
		return selectSuccess(entry), nil
	}

	// Check if this status code is available for the operation.
	if !hasStatusCode(entry, code) {
		return nil, fmt.Errorf("%w: %d (available: %s)",
			ErrStatusNotAvailable, code, formatAvailableCodes(entry))
	}

	return &SelectedScenario{
		StatusCode: code,
		Schema:     entry.ResponseSchemas[code],
	}, nil
}

// selectSuccess returns the success scenario with the entry's SuccessCode
// and corresponding response schema. Falls back to the default response
// schema (key 0) per Decision #31.
func selectSuccess(entry *model.BehaviorEntry) *SelectedScenario {
	schema := entry.ResponseSchemas[entry.SuccessCode]
	if schema == nil {
		schema = entry.ResponseSchemas[0]
	}

	return &SelectedScenario{
		StatusCode: entry.SuccessCode,
		Schema:     schema,
	}
}

// hasStatusCode returns true if the entry has the given code in its
// ResponseSchemas (key presence means "defined in spec").
func hasStatusCode(entry *model.BehaviorEntry, code int) bool {
	_, ok := entry.ResponseSchemas[code]

	return ok
}

// formatAvailableCodes returns a sorted, comma-separated list of all
// available status codes for the operation.
func formatAvailableCodes(entry *model.BehaviorEntry) string {
	seen := make(map[int]struct{})
	seen[entry.SuccessCode] = struct{}{}

	for code := range entry.ResponseSchemas {
		if code != 0 { // skip default schema sentinel
			seen[code] = struct{}{}
		}
	}

	codes := make([]int, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}

	sort.Ints(codes)

	parts := make([]string, len(codes))
	for i, code := range codes {
		parts[i] = strconv.Itoa(code)
	}

	return strings.Join(parts, ", ")
}
