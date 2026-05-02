package router

import (
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// summarizeValidationError produces a bounded, human-readable summary of a
// schema validation error. It extracts the first leaf violation as a sample
// (with path + got/want), reports the total violation count, and appends a
// static fix hint explaining why null fields appear in circular schemas.
//
// The output format is structured for slog key=value parsing:
//
//	violations=N error="/path: description" fix="..."
func summarizeValidationError(err *jsonschema.ValidationError) string {
	var first *jsonschema.ValidationError

	count := countLeafViolations(err, &first)

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("violations=%d", count))

	if first != nil {
		sb.WriteString(fmt.Sprintf(" error=%q", formatLeafError(first)))
	}

	sb.WriteString(` fix="increase --max-depth for deeper nesting, or restructure the circular $ref in the spec"`)

	return sb.String()
}

// countLeafViolations recursively counts leaf nodes (violations with no causes)
// in the validation error tree and captures the first one encountered.
func countLeafViolations(err *jsonschema.ValidationError, first **jsonschema.ValidationError) int {
	if len(err.Causes) == 0 {
		if *first == nil {
			*first = err
		}

		return 1
	}

	total := 0
	for _, cause := range err.Causes {
		total += countLeafViolations(cause, first)
	}

	return total
}

// formatLeafError formats a single leaf validation error as "/<path>: <description>".
func formatLeafError(err *jsonschema.ValidationError) string {
	path := jsonPtr(err.InstanceLocation)
	if path == "" {
		path = "/"
	}

	desc := describeError(err)

	return fmt.Sprintf("%s: %s", path, desc)
}

// describeError extracts the human-readable error description from a ValidationError.
func describeError(err *jsonschema.ValidationError) string {
	if err.ErrorKind == nil {
		return ""
	}

	p := message.NewPrinter(language.English)

	return err.ErrorKind.LocalizedString(p)
}

// jsonPtr encodes instance location tokens as a JSON Pointer (RFC 6901).
func jsonPtr(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, tok := range tokens {
		sb.WriteByte('/')
		sb.WriteString(tok)
	}

	return sb.String()
}
