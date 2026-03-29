// Scenario inference maps behavior types to their expected response scenarios.
//
// Each behavior type has a fixed set of scenarios that represent the possible
// response outcomes. The Scenario Router (Task 10) uses these to determine
// which response to generate for a given request.
//
// This is a pure lookup — no heuristics, no schema inspection. The mapping
// is based on REST semantics:
//
//   - create: success (201) or validation error (400/422)
//   - fetch:  success (200) or not found (404)
//   - list:   success (200) — empty lists are success, not errors
//   - update: success (200) or validation error or not found
//   - delete: success (200/204) or not found (404)
//   - generic: success only — no assumptions about error modes

package classifier

import "github.com/mimikos-io/mimikos/internal/model"

// scenarioMap is the behavior-type-to-scenarios lookup table.
//
//nolint:gochecknoglobals // immutable lookup table
var scenarioMap = map[model.BehaviorType][]model.Scenario{
	model.BehaviorCreate:  {model.ScenarioSuccess, model.ScenarioValidationError},
	model.BehaviorFetch:   {model.ScenarioSuccess, model.ScenarioNotFound},
	model.BehaviorList:    {model.ScenarioSuccess},
	model.BehaviorUpdate:  {model.ScenarioSuccess, model.ScenarioValidationError, model.ScenarioNotFound},
	model.BehaviorDelete:  {model.ScenarioSuccess, model.ScenarioNotFound},
	model.BehaviorGeneric: {model.ScenarioSuccess},
}

// InferScenarios returns the expected response scenarios for a behavior type.
// The returned slice is a copy and safe to modify.
// Unknown behavior types fall back to success-only.
func InferScenarios(bt model.BehaviorType) []model.Scenario {
	if scenarios, ok := scenarioMap[bt]; ok {
		// Return a copy to prevent callers from mutating the shared slice.
		result := make([]model.Scenario, len(scenarios))
		copy(result, scenarios)

		return result
	}

	return []model.Scenario{model.ScenarioSuccess}
}
