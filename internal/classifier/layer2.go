// Layer 2: Response schema signals.
//
// Layer 2 refines the Layer 1 classification using response metadata:
//
//   - Array response: if the 200 response schema type is "array", confirms list.
//   - Status 201: reinforces create classification (confidence boost).
//   - Status 204: reinforces delete or action classification (confidence boost).
//
// Layer 2 only boosts confidence — it never overrides the behavior type.
// Status codes and explicit array types are clean, unambiguous signals that
// confirm L1 without risk of regression.
//
// Singleton detection (overriding list → fetch for GET /me, /account, etc.)
// was evaluated and deferred — schema-based detection proved unreliable:
// most objects have incidental array properties (images, tags) that produce
// false list signals, and many list wrappers have unresolvable property
// schemas. Detecting that a response IS an array is easy and reliable;
// proving it is NOT a list (singleton) is hard. Singletons remain a known
// L1 limitation for Session 13.

package classifier

import (
	"net/http"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// applyLayer2 refines the L1 result using response schema and status code signals.
func (c *Classifier) applyLayer2(op parser.Operation, result Result) Result {
	result = c.applyArrayConfirmation(op, result)
	result = c.applyStatusCodeSignals(op, result)

	return result
}

// applyArrayConfirmation boosts confidence when the 200 response schema is
// explicitly typed as "array", confirming a list classification.
func (c *Classifier) applyArrayConfirmation(op parser.Operation, result Result) Result {
	if result.Type != model.BehaviorList {
		return result
	}

	schema := responseSchema(op, http.StatusOK)
	if schema == nil {
		return result
	}

	if parser.PrimaryType(schema.Raw) == "array" {
		result.Confidence = min(result.Confidence+confidenceBoostL2, confidenceMax)
	}

	return result
}

// applyStatusCodeSignals boosts confidence when response status codes
// confirm the L1 classification.
func (c *Classifier) applyStatusCodeSignals(op parser.Operation, result Result) Result {
	if op.Responses == nil {
		return result
	}

	// Status 201 confirms create.
	if result.Type == model.BehaviorCreate {
		if _, has201 := op.Responses[201]; has201 {
			result.Confidence = min(result.Confidence+confidenceBoostL2, confidenceMax)
		}
	}

	// Status 204 confirms delete or generic (action).
	if result.Type == model.BehaviorDelete || result.Type == model.BehaviorGeneric {
		if _, has204 := op.Responses[204]; has204 {
			result.Confidence = min(result.Confidence+confidenceBoostL2, confidenceMax)
		}
	}

	return result
}

// responseSchema extracts the SchemaRef for the given status code from the
// operation's responses. Returns nil if the response or schema is not defined.
func responseSchema(op parser.Operation, statusCode int) *parser.SchemaRef {
	if op.Responses == nil {
		return nil
	}

	resp, ok := op.Responses[statusCode]
	if !ok || resp == nil {
		return nil
	}

	return resp.Schema
}
