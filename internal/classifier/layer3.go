// Layer 3: operationId keyword hints.
//
// Layer 3 tokenizes the operationId string and scans for CRUD keywords:
//
//   - create/add/new/insert    -> create signal
//   - get/fetch/retrieve/read  -> fetch signal
//   - list/search/find/browse  -> list signal
//   - update/edit/modify/patch -> update signal
//   - delete/remove/destroy    -> delete signal
//
// Layer 3 is supplementary — it boosts confidence when the keyword confirms
// the current classification, and can override only when confidence is weak
// (<= 0.4). Its primary use case is POST-to-item disambiguation where the
// operationId contains "update"/"edit" keywords.
//
// The first token is skipped if it matches the HTTP method of the operation
// (e.g., "Get" in "GetChargesCharge" for a GET request), since it carries
// no CRUD semantic beyond what Layer 1 already knows.

package classifier

import (
	"strings"
	"unicode"

	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/mimikos-io/mimikos/internal/parser"
)

// crudKeywords maps operationId tokens to their inferred behavior type.
//
// Note: "add", "remove", "insert", and "search" also appear in actionVerbs
// (layer1.go). This overlap is safe because L1 action detection produces
// confidenceModerate (0.6), and L3 only overrides at confidenceWeak (0.4).
// The confidence gating prevents L3 from undoing L1's action classification.
//
//nolint:gochecknoglobals // immutable lookup table
var crudKeywords = map[string]model.BehaviorType{
	"create":   model.BehaviorCreate,
	"add":      model.BehaviorCreate,
	"new":      model.BehaviorCreate,
	"insert":   model.BehaviorCreate,
	"get":      model.BehaviorFetch,
	"fetch":    model.BehaviorFetch,
	"retrieve": model.BehaviorFetch,
	"read":     model.BehaviorFetch,
	"list":     model.BehaviorList,
	"search":   model.BehaviorList,
	"find":     model.BehaviorList,
	"browse":   model.BehaviorList,
	"update":   model.BehaviorUpdate,
	"edit":     model.BehaviorUpdate,
	"modify":   model.BehaviorUpdate,
	"patch":    model.BehaviorUpdate,
	"delete":   model.BehaviorDelete,
	"remove":   model.BehaviorDelete,
	"destroy":  model.BehaviorDelete,
	"drop":     model.BehaviorDelete,
}

// applyLayer3 refines the current result using operationId keyword signals.
func (c *Classifier) applyLayer3(op parser.Operation, result Result) Result {
	if op.OperationID == "" {
		return result
	}

	signal, ok := extractKeywordSignal(op)
	if !ok {
		return result
	}

	if signal == result.Type {
		// Keyword confirms current classification — boost confidence.
		result.Confidence = min(result.Confidence+confidenceBoostL3, confidenceMax)

		return result
	}

	// Keyword disagrees with current classification.
	// Only override when current confidence is weak.
	if result.Confidence <= confidenceWeak {
		result.Type = signal
		result.Confidence = confidenceL3
	}

	return result
}

// extractKeywordSignal tokenizes the operationId and returns the behavior type
// of the first matched CRUD keyword (left-to-right scan). First-match-wins is
// intentional — real-world operationIds contain at most one CRUD verb.
// Returns false if no keyword matches.
func extractKeywordSignal(op parser.Operation) (model.BehaviorType, bool) {
	tokens := TokenizeOperationID(op.OperationID)

	methodLower := strings.ToLower(op.Method)
	skipFirst := len(tokens) > 0 && tokens[0] == methodLower

	for i, token := range tokens {
		// Skip the first token if it matches the HTTP method.
		if i == 0 && skipFirst {
			continue
		}

		if bt, ok := crudKeywords[token]; ok {
			return bt, true
		}
	}

	return "", false
}

// TokenizeOperationID splits an operationId into lowercase tokens by
// camelCase boundaries, hyphens, underscores, and slashes.
//
// Examples:
//
//	"createUser"                  -> ["create", "user"]
//	"get-current-users-profile"   -> ["get", "current", "users", "profile"]
//	"PostAccountsAccount"         -> ["post", "accounts", "account"]
//	"listAPIKeys"                 -> ["list", "api", "keys"]
func TokenizeOperationID(operationID string) []string {
	if operationID == "" {
		return nil
	}

	var tokens []string

	var current strings.Builder

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, strings.ToLower(current.String()))
			current.Reset()
		}
	}

	runes := []rune(operationID)
	for i, r := range runes {
		switch {
		case r == '-' || r == '_' || r == '/':
			flush()
		case unicode.IsUpper(r):
			// Start a new token on case transitions:
			// - lowercase→uppercase: "create|U|ser"
			// - uppercase→uppercase when next is lowercase: "API|K|eys"
			if current.Len() > 0 {
				prevIsUpper := i > 0 && unicode.IsUpper(runes[i-1])
				nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])

				if !prevIsUpper || nextIsLower {
					flush()
				}
			}

			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}

	flush()

	return tokens
}
