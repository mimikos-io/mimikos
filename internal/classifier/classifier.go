// Package classifier implements the layered heuristic that infers CRUD
// behavior types from OpenAPI operation metadata. It is Mimikos' core
// differentiator.
//
// The classifier uses a layered approach:
//   - Layer 1: HTTP method + path pattern (collection vs item, sub-resource delete)
//   - Layer 2: Response schema signals (status codes, array vs object)
//   - Layer 3: operationId + summary keyword hints (with targeted list↔fetch override)
//   - Fallback: generic with low confidence
package classifier

import (
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// Confidence levels for classifications.
const (
	confidenceStrong   = 0.8  // Strong match: unambiguous method + path pattern.
	confidenceModerate = 0.6  // Moderate: plausible but could be overridden by later layers.
	confidenceWeak     = 0.4  // Weak: fallback or ambiguous pattern.
	confidenceL3       = 0.6  // L3 override: supplementary operationId signal.
	confidenceBoostL2  = 0.1  // L2 confirmation boost (capped at 0.9).
	confidenceBoostL3  = 0.05 // L3 confirmation boost.
	confidenceMax      = 0.9  // Maximum confidence after boosts.
)

// Result is the classifier's output for a single operation.
type Result struct {
	// Type is the inferred CRUD behavior.
	Type model.BehaviorType

	// Confidence is the classifier's confidence in the classification (0.0-1.0).
	Confidence float64
}

// ConfidenceLabel maps a confidence score to a human-readable label.
// Thresholds align with the classifier's internal confidence constants:
// strong (>=0.8) → "high", moderate (>=0.6) → "medium", weak (<0.6) → "low".
func ConfidenceLabel(c float64) string {
	switch {
	case c >= confidenceStrong:
		return "high"
	case c >= confidenceModerate:
		return "medium"
	default:
		return "low"
	}
}

// Classifier infers CRUD behavior types from OpenAPI operation metadata
// using a layered heuristic approach. The pipeline runs L1 -> L2 -> L3
// sequentially, with each layer able to confirm, override, or pass
// through the previous result based on confidence gating.
type Classifier struct{}

// New creates a new Classifier.
func New() *Classifier {
	return &Classifier{}
}

// Classify infers the behavior type for a single OpenAPI operation
// by running it through the L1 -> L2 -> L3 classification pipeline.
func (c *Classifier) Classify(op parser.Operation) Result {
	result := c.applyLayer1(op)
	result = c.applyLayer2(op, result)
	result = c.applyLayer3(op, result)

	return result
}
