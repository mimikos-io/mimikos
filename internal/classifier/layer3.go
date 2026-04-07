// Layer 3: operationId + summary keyword hints.
//
// Layer 3 tokenizes the operationId and summary strings and scans for CRUD
// keywords:
//
//   - create/add/new/insert       -> create signal
//   - get/fetch/retrieve/read/show -> fetch signal
//   - list/search/find/browse      -> list signal
//   - update/edit/modify/patch     -> update signal
//   - delete/remove/destroy/drop   -> delete signal
//
// Signal priority: operationId first (full scan with method-skip), summary
// fallback (first token only with method-skip). Summary is restricted to
// first-token-only to avoid noun/verb collisions (e.g., "show" as a TV show).
//
// Override rules:
//   - General: any signal can override at weak confidence (<= 0.4)
//   - Targeted: operationId signals at position > 0 (after a namespace
//     prefix) can override list<->fetch at strong confidence (<= 0.8)
//
// The first token is skipped if it matches the HTTP method (e.g., "Get" in
// "GetChargesCharge" for GET, or "Get" in summary "Get the user" for GET).

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
	"show":     model.BehaviorFetch,
	"delete":   model.BehaviorDelete,
	"remove":   model.BehaviorDelete,
	"destroy":  model.BehaviorDelete,
	"drop":     model.BehaviorDelete,
}

// applyLayer3 refines the current result using operationId and summary keyword signals.
//
// Signal priority: operationId first (with HTTP method-skip), summary fallback
// (first token only, with method-skip). Summary text is natural language —
// only the leading verb is checked to avoid noun/verb collisions (e.g., "show"
// as a TV show, "browse" as a section name).
//
// Override rules:
//   - General: any signal can override at weak confidence (<= 0.4)
//   - Targeted list<->fetch at strong confidence (<= 0.8):
//     a) operationId keyword at position > 0 (namespace-prefixed)
//     b) summary keyword that is unambiguously fetch-only ("show", "fetch")
//
// "retrieve" is excluded from (b) because it is used for both singletons
// ("Retrieve account") and lists ("Retrieve all API Keys", "Retrieve comments").
func (c *Classifier) applyLayer3(op parser.Operation, result Result) Result {
	opIDSignal, opIDPos := extractOperationIDSignal(op)
	hasOpID := opIDPos >= 0
	summarySignal, summaryKeyword, hasSummary := extractSummarySignal(op)

	// Use operationId signal if available, otherwise fallback to summary.
	signal := opIDSignal

	if !hasOpID {
		if !hasSummary {
			return result
		}

		signal = summarySignal
	}

	if signal == result.Type {
		result.Confidence = min(result.Confidence+confidenceBoostL3, confidenceMax)

		return result
	}

	// General override: any signal source can override weak confidence.
	if result.Confidence <= confidenceWeak {
		result.Type = signal
		result.Confidence = confidenceL3

		return result
	}

	// Targeted list<->fetch override at strong confidence.
	if result.Confidence <= confidenceStrong && isListFetchTransition(result.Type, signal) {
		eligible := false

		// (a) operationId keyword at position > 0 (after namespace prefix).
		if hasOpID && opIDPos > 0 {
			eligible = true
		}

		// (b) Summary keyword that is unambiguously fetch-only.
		// "show" and "fetch" are never used for collection endpoints.
		// "retrieve" is excluded — used for both singletons and lists.
		// Checked even when operationId produced the signal, since the
		// operationId may be at position 0 (excluded from path a) while
		// the summary provides an independent safe signal.
		if !eligible && hasSummary && isSafeSummaryOverride(summaryKeyword) {
			eligible = true
		}

		if eligible {
			result.Type = signal
			result.Confidence = confidenceL3
		}
	}

	return result
}

// safeSummaryKeywords are summary-first-token keywords that are unambiguous
// enough to trigger the targeted list→fetch override. "show" and "fetch" are
// only used for single-resource endpoints across the corpus. "retrieve" is
// excluded because it's used for both singletons and collections (e.g.,
// SendGrid "Retrieve all API Keys", Notion "Retrieve comments").
//
//nolint:gochecknoglobals // immutable lookup set
var safeSummaryKeywords = map[string]struct{}{
	"show":  {},
	"fetch": {},
}

// isSafeSummaryOverride returns true if the summary keyword is unambiguous
// enough to trigger the targeted list→fetch override at strong confidence.
func isSafeSummaryOverride(keyword string) bool {
	_, ok := safeSummaryKeywords[keyword]

	return ok
}

// isListFetchTransition returns true when the current type and signal form
// a list<->fetch pair — the only transition allowed at strong confidence.
func isListFetchTransition(current model.BehaviorType, signal model.BehaviorType) bool {
	return (current == model.BehaviorList && signal == model.BehaviorFetch) ||
		(current == model.BehaviorFetch && signal == model.BehaviorList)
}

// extractOperationIDSignal tokenizes the operationId and returns the behavior type
// of the first matched CRUD keyword (left-to-right scan) and its token position.
// First-match-wins is intentional — real-world operationIds contain at most one
// CRUD verb.
//
// The first token is skipped if it matches the HTTP method (e.g., "Get" in
// "GetChargesCharge" for a GET request), since operationId conventionally
// prefixes the method name.
//
// Returns position -1 if no keyword matches or operationId is empty.
// Position 0 means the keyword is the first token (possibly a method synonym
// like "retrieve" or "find"); position > 0 means it follows a namespace
// prefix (e.g., "apps/get-authenticated" → "get" at position 1).
func extractOperationIDSignal(op parser.Operation) (model.BehaviorType, int) {
	if op.OperationID == "" {
		return "", -1
	}

	tokens := Tokenize(op.OperationID)

	skipFirst := len(tokens) > 0 && tokens[0] == strings.ToLower(op.Method)

	for i, token := range tokens {
		if i == 0 && skipFirst {
			continue
		}

		if bt, ok := crudKeywords[token]; ok {
			return bt, i
		}
	}

	return "", -1
}

// extractSummarySignal checks only the first token of the operation summary
// for a CRUD keyword. Unlike operationId (which scans all tokens), summary
// text is natural language where only the leading verb conveys intent —
// subsequent words may be nouns that collide with keywords (e.g., "show" as
// a TV show in "Get Show Episodes", "browse" as a section in "Get a Browse
// Category").
//
// If the first token matches the HTTP method (e.g., "Get" on a GET endpoint),
// no signal is returned — "Get" is ambiguous between singletons ("Get Current
// User's Profile") and lists ("Get Followed Artists"). Non-method verbs like
// "Retrieve", "Show", "List" at position zero are unambiguous intent markers.
//
// Returns the behavior type, the matched keyword, and true if a signal was
// found. Returns ("", "", false) if no keyword matches, summary is empty, or
// the only token is the HTTP method.
func extractSummarySignal(op parser.Operation) (model.BehaviorType, string, bool) {
	if op.Summary == "" {
		return "", "", false
	}

	tokens := Tokenize(op.Summary)
	if len(tokens) == 0 {
		return "", "", false
	}

	first := tokens[0]

	// Skip if the first token matches the HTTP method — ambiguous.
	if first == strings.ToLower(op.Method) {
		return "", "", false
	}

	if bt, ok := crudKeywords[first]; ok {
		return bt, first, true
	}

	return "", "", false
}

// Tokenize splits a string into lowercase tokens by camelCase boundaries,
// hyphens, underscores, slashes, and spaces.
//
// Examples:
//
//	"createUser"                  -> ["create", "user"]
//	"get-current-users-profile"   -> ["get", "current", "users", "profile"]
//	"PostAccountsAccount"         -> ["post", "accounts", "account"]
//	"listAPIKeys"                 -> ["list", "api", "keys"]
//	"Update a customer"           -> ["update", "a", "customer"]
func Tokenize(s string) []string {
	if s == "" {
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

	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r == '-' || r == '_' || r == '/' || r == ' ':
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
