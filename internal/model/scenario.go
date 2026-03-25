package model

// Scenario represents a response scenario that an operation can produce.
type Scenario string

const (
	// ScenarioSuccess is the happy-path response.
	ScenarioSuccess Scenario = "success"
	// ScenarioValidationError is returned when request validation fails.
	ScenarioValidationError Scenario = "validation_error"
	// ScenarioNotFound is returned when the target resource does not exist.
	ScenarioNotFound Scenario = "not_found"
)

//nolint:gochecknoglobals // immutable lookup table for scenario validation
var validScenarios = map[Scenario]struct{}{
	ScenarioSuccess:         {},
	ScenarioValidationError: {},
	ScenarioNotFound:        {},
}

// String returns the string representation of the scenario.
func (s Scenario) String() string {
	return string(s)
}

// IsValid returns true if the scenario is one of the defined constants.
func (s Scenario) IsValid() bool {
	_, ok := validScenarios[s]

	return ok
}
