package router

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"github.com/stretchr/testify/assert"
)

// --- Validation error summarization tests (Task 52) ---

func TestSummarizeValidationError_SingleViolation(t *testing.T) {
	t.Parallel()

	err := &jsonschema.ValidationError{
		InstanceLocation: []string{"0", "name"},
		ErrorKind:        &kind.Type{Got: "null", Want: []string{"string"}},
	}

	summary := summarizeValidationError(err)

	assert.Contains(t, summary, "violations=1")
	assert.Contains(t, summary, "/0/name")
	assert.Contains(t, summary, "null")
}

func TestSummarizeValidationError_ManyViolations(t *testing.T) {
	t.Parallel()

	// Simulate a deeply nested circular schema producing many violations.
	// Root has 3 children, each a leaf violation.
	err := &jsonschema.ValidationError{
		InstanceLocation: []string{},
		ErrorKind:        &kind.Schema{Location: "test.json"},
		Causes: []*jsonschema.ValidationError{
			{
				InstanceLocation: []string{"0", "category", "parent"},
				ErrorKind:        &kind.Type{Got: "null", Want: []string{"object"}},
			},
			{
				InstanceLocation: []string{"0", "category", "parent", "parent"},
				ErrorKind:        &kind.Type{Got: "null", Want: []string{"object"}},
			},
			{
				InstanceLocation: []string{"0", "category", "children", "0"},
				ErrorKind:        &kind.Type{Got: "null", Want: []string{"object"}},
			},
		},
	}

	summary := summarizeValidationError(err)

	// Should contain the violation count.
	assert.Contains(t, summary, "violations=3")
	// Should contain one sample error with path and got/want.
	assert.Contains(t, summary, "/0/category/parent")
	// Should contain the actionable hint.
	assert.Contains(t, summary, "circular")
	assert.Contains(t, summary, "--max-depth")
}

func TestSummarizeValidationError_ContainsFixGuidance(t *testing.T) {
	t.Parallel()

	err := &jsonschema.ValidationError{
		InstanceLocation: []string{"data", "nested"},
		ErrorKind:        &kind.Type{Got: "null", Want: []string{"object"}},
	}

	summary := summarizeValidationError(err)

	// The summary must be self-contained with fix guidance.
	assert.Contains(t, summary, "fix=")
	assert.Contains(t, summary, "--max-depth")
}
