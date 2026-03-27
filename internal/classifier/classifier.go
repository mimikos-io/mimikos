// Package classifier implements the layered heuristic that infers CRUD
// behavior types from OpenAPI operation metadata. It is Mimikos' core
// differentiator.
//
// The classifier uses a layered approach:
//   - Layer 1: HTTP method + path pattern (collection vs item)
//   - Layer 2: Response schema signals (status codes, array vs object) [Session 12]
//   - Layer 3: operationId keyword hints [Session 12]
//   - Fallback: generic with low confidence [Session 12]
package classifier

import (
	"net/http"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// Confidence levels for Layer 1 classifications.
const (
	confidenceStrong   = 0.8 // Strong match: unambiguous method + path pattern.
	confidenceModerate = 0.6 // Moderate: plausible but could be overridden by later layers.
	confidenceWeak     = 0.4 // Weak: fallback or ambiguous pattern.
)

// Result is the classifier's output for a single operation.
type Result struct {
	// Type is the inferred CRUD behavior.
	Type model.BehaviorType

	// Confidence is the classifier's confidence in the classification (0.0–1.0).
	Confidence float64
}

// Classifier infers CRUD behavior types from OpenAPI operation metadata
// using a layered heuristic approach. Currently stateless (Layer 1 only).
// Layers 2/3 (Session 12) will add fields for operationId patterns and
// schema signal configuration.
type Classifier struct{}

// New creates a new Classifier.
func New() *Classifier {
	return &Classifier{}
}

// Classify infers the behavior type for a single OpenAPI operation.
func (c *Classifier) Classify(op parser.Operation) Result {
	return c.classifyLayer1(op)
}

// classifyLayer1 classifies based on HTTP method and path structure.
func (c *Classifier) classifyLayer1(op parser.Operation) Result {
	info := analyzePath(op.Path)

	// Action verbs always produce generic, regardless of method.
	if info.isAction {
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
	}

	switch op.Method {
	case http.MethodGet:
		return c.classifyGET(info)
	case http.MethodPost:
		return c.classifyPOST(info)
	case http.MethodPut:
		return c.classifyPUT(info)
	case http.MethodPatch:
		return c.classifyPATCH(info)
	case http.MethodDelete:
		return c.classifyDELETE(info)
	default:
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceWeak}
	}
}

func (c *Classifier) classifyGET(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorFetch, Confidence: confidenceStrong}
	}

	return Result{Type: model.BehaviorList, Confidence: confidenceStrong}
}

func (c *Classifier) classifyPOST(info pathInfo) Result {
	if info.isItem {
		// POST to item path: could be update (Stripe/Twilio) or action (GitHub).
		// Layer 1 cannot distinguish — classify as generic, defer to L2/L3.
		return Result{Type: model.BehaviorGeneric, Confidence: confidenceWeak}
	}

	return Result{Type: model.BehaviorCreate, Confidence: confidenceStrong}
}

func (c *Classifier) classifyPUT(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorUpdate, Confidence: confidenceStrong}
	}

	// PUT to collection: bulk operation (e.g., PUT /me/albums on Spotify).
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}

func (c *Classifier) classifyPATCH(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorUpdate, Confidence: confidenceStrong}
	}

	// PATCH to collection: unusual, treat as generic.
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}

func (c *Classifier) classifyDELETE(info pathInfo) Result {
	if info.isItem {
		return Result{Type: model.BehaviorDelete, Confidence: confidenceStrong}
	}

	// DELETE on collection: bulk delete (e.g., DELETE /me/albums on Spotify).
	return Result{Type: model.BehaviorGeneric, Confidence: confidenceModerate}
}
