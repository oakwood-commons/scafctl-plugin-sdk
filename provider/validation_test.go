// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSchemaValidator(t *testing.T) {
	sv := NewSchemaValidator()
	assert.NotNil(t, sv)
}

func TestValidationError_Error(t *testing.T) {
	t.Run("without actual/expected", func(t *testing.T) {
		e := &ValidationError{Field: "name", Message: "is required"}
		assert.Equal(t, "name: is required", e.Error())
	})

	t.Run("with actual/expected", func(t *testing.T) {
		e := &ValidationError{Field: "age", Message: "wrong type", Actual: "string", Expected: "integer"}
		assert.Equal(t, "age: wrong type (actual: string, expected: integer)", e.Error())
	})
}

func TestValidationErrors_Error(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var errs ValidationErrors
		assert.Empty(t, errs.Error())
	})

	t.Run("single error", func(t *testing.T) {
		errs := ValidationErrors{&ValidationError{Field: "name", Message: "is required"}}
		assert.Equal(t, "name: is required", errs.Error())
	})

	t.Run("multiple errors", func(t *testing.T) {
		errs := ValidationErrors{
			&ValidationError{Field: "name", Message: "is required"},
			&ValidationError{Field: "age", Message: "must be positive"},
		}
		result := errs.Error()
		assert.Contains(t, result, "validation failed with 2 errors")
		assert.Contains(t, result, "name: is required")
		assert.Contains(t, result, "age: must be positive")
	})
}

func TestSchemaValidator_ValidateInputs(t *testing.T) {
	sv := NewSchemaValidator()

	t.Run("nil schema passes", func(t *testing.T) {
		assert.NoError(t, sv.ValidateInputs(map[string]any{"x": 1}, nil))
	})

	t.Run("valid inputs pass", func(t *testing.T) {
		schema := &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {Type: "string"},
			},
			Required: []string{"name"},
		}
		assert.NoError(t, sv.ValidateInputs(map[string]any{"name": "hello"}, schema))
	})

	t.Run("missing required field fails", func(t *testing.T) {
		schema := &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {Type: "string"},
			},
			Required: []string{"name"},
		}
		err := sv.ValidateInputs(map[string]any{}, schema)
		require.Error(t, err)
	})
}

func TestSchemaValidator_ValidateOutput(t *testing.T) {
	sv := NewSchemaValidator()

	t.Run("nil schema passes", func(t *testing.T) {
		assert.NoError(t, sv.ValidateOutput(map[string]any{"x": 1}, nil))
	})

	t.Run("map output", func(t *testing.T) {
		schema := &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"result": {Type: "string"},
			},
		}
		assert.NoError(t, sv.ValidateOutput(map[string]any{"result": "ok"}, schema))
	})

	t.Run("nil output", func(t *testing.T) {
		schema := &jsonschema.Schema{Type: "object"}
		assert.NoError(t, sv.ValidateOutput(nil, schema))
	})

	t.Run("non-map output wraps in value", func(t *testing.T) {
		schema := &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"value": {Type: "string"},
			},
		}
		assert.NoError(t, sv.ValidateOutput("hello", schema))
	})
}
