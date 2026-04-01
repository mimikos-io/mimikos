package router

import "github.com/mimikos-io/mimikos/internal/model"

// SelectedScenario is the output of scenario selection: which scenario
// to serve, with what status code, and which schema to generate from.
type SelectedScenario struct {
	// Scenario is the selected response scenario (always success in MVP
	// deterministic mode — validation errors are handled before reaching
	// scenario selection).
	Scenario model.Scenario

	// StatusCode is the HTTP status code to return.
	StatusCode int

	// Schema is the compiled response schema to generate data from, or nil
	// if the operation defines no response schema for this status code.
	Schema *model.CompiledSchema
}

// SelectScenario picks the success response scenario for a matched operation.
// It returns the entry's SuccessCode and the corresponding response schema.
// If no schema exists for the SuccessCode, the default response schema (key 0)
// is used as fallback per Decision #31.
//
// This function is only called for valid requests. Invalid requests
// short-circuit to the error responder before reaching scenario selection.
func SelectScenario(entry *model.BehaviorEntry) *SelectedScenario {
	schema := entry.ResponseSchemas[entry.SuccessCode]
	if schema == nil {
		schema = entry.ResponseSchemas[0]
	}

	return &SelectedScenario{
		Scenario:   model.ScenarioSuccess,
		StatusCode: entry.SuccessCode,
		Schema:     schema,
	}
}
