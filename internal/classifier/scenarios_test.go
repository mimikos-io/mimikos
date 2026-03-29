package classifier_test

import (
	"testing"

	"github.com/mimikos-io/mimikos/internal/classifier"
	"github.com/mimikos-io/mimikos/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestInferScenarios(t *testing.T) {
	tests := []struct {
		name     string
		behavior model.BehaviorType
		want     []model.Scenario
	}{
		{
			name:     "create has success and validation_error",
			behavior: model.BehaviorCreate,
			want:     []model.Scenario{model.ScenarioSuccess, model.ScenarioValidationError},
		},
		{
			name:     "fetch has success and not_found",
			behavior: model.BehaviorFetch,
			want:     []model.Scenario{model.ScenarioSuccess, model.ScenarioNotFound},
		},
		{
			name:     "list has success only",
			behavior: model.BehaviorList,
			want:     []model.Scenario{model.ScenarioSuccess},
		},
		{
			name:     "update has success validation_error and not_found",
			behavior: model.BehaviorUpdate,
			want:     []model.Scenario{model.ScenarioSuccess, model.ScenarioValidationError, model.ScenarioNotFound},
		},
		{
			name:     "delete has success and not_found",
			behavior: model.BehaviorDelete,
			want:     []model.Scenario{model.ScenarioSuccess, model.ScenarioNotFound},
		},
		{
			name:     "generic has success only",
			behavior: model.BehaviorGeneric,
			want:     []model.Scenario{model.ScenarioSuccess},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifier.InferScenarios(tt.behavior)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInferScenarios_UnknownType_ReturnsSuccessOnly(t *testing.T) {
	got := classifier.InferScenarios(model.BehaviorType("unknown"))
	assert.Equal(t, []model.Scenario{model.ScenarioSuccess}, got,
		"unknown behavior type should fall back to success-only")
}
