// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapability_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		cap      Capability
		expected bool
	}{
		{name: "from", cap: CapabilityFrom, expected: true},
		{name: "transform", cap: CapabilityTransform, expected: true},
		{name: "validation", cap: CapabilityValidation, expected: true},
		{name: "authentication", cap: CapabilityAuthentication, expected: true},
		{name: "action", cap: CapabilityAction, expected: true},
		{name: "invalid", cap: Capability("invalid"), expected: false},
		{name: "empty", cap: Capability(""), expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cap.IsValid())
		})
	}
}

func TestCapability_String(t *testing.T) {
	assert.Equal(t, "from", CapabilityFrom.String())
	assert.Equal(t, "transform", CapabilityTransform.String())
}

func TestDescriptor_IsSensitiveField(t *testing.T) {
	d := &Descriptor{SensitiveFields: []string{"password", "token"}}
	assert.True(t, d.IsSensitiveField("password"))
	assert.True(t, d.IsSensitiveField("token"))
	assert.False(t, d.IsSensitiveField("username"))

	empty := &Descriptor{}
	assert.False(t, empty.IsSensitiveField("anything"))
}

func TestDescriptor_DescribeWhatIf(t *testing.T) {
	ctx := context.Background()

	t.Run("no WhatIf function", func(t *testing.T) {
		d := &Descriptor{Name: "test-provider"}
		result := d.DescribeWhatIf(ctx, nil)
		assert.Equal(t, "Would execute test-provider provider", result)
	})

	t.Run("WhatIf returns value", func(t *testing.T) {
		d := &Descriptor{
			Name: "test-provider",
			WhatIf: func(_ context.Context, _ any) (string, error) {
				return "Would do something specific", nil
			},
		}
		result := d.DescribeWhatIf(ctx, nil)
		assert.Equal(t, "Would do something specific", result)
	})

	t.Run("WhatIf returns empty string", func(t *testing.T) {
		d := &Descriptor{
			Name: "test-provider",
			WhatIf: func(_ context.Context, _ any) (string, error) {
				return "", nil
			},
		}
		result := d.DescribeWhatIf(ctx, nil)
		assert.Equal(t, "Would execute test-provider provider", result)
	})

	t.Run("WhatIf returns error", func(t *testing.T) {
		d := &Descriptor{
			Name: "test-provider",
			WhatIf: func(_ context.Context, _ any) (string, error) {
				return "", errors.New("whatif failed")
			},
		}
		result := d.DescribeWhatIf(ctx, nil)
		assert.Equal(t, "Would execute test-provider provider", result)
	})
}

func validDescriptor() *Descriptor {
	return &Descriptor{
		Name:         "test",
		APIVersion:   "v1",
		Version:      semver.MustParse("1.0.0"),
		Description:  "A test provider",
		Capabilities: []Capability{CapabilityTransform},
		Schema:       &jsonschema.Schema{Type: "object"},
		OutputSchemas: map[Capability]*jsonschema.Schema{
			CapabilityTransform: {Type: "object"},
		},
	}
}

func TestValidateDescriptor(t *testing.T) {
	t.Run("valid descriptor", func(t *testing.T) {
		require.NoError(t, ValidateDescriptor(validDescriptor()))
	})

	t.Run("nil descriptor", func(t *testing.T) {
		err := ValidateDescriptor(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "descriptor is nil")
	})

	t.Run("no capabilities", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = nil
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one capability")
	})

	t.Run("no output schemas", func(t *testing.T) {
		d := validDescriptor()
		d.OutputSchemas = nil
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must define OutputSchemas")
	})

	t.Run("missing output schema for capability", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityTransform, CapabilityAction}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing output schema for capability")
	})

	t.Run("validation requires valid and errors", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityValidation}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityValidation: {Type: "object", Properties: map[string]*jsonschema.Schema{}},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires output field")
	})

	t.Run("validation with correct fields", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityValidation}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityValidation: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"valid":  {Type: "boolean"},
					"errors": {Type: "array"},
				},
			},
		}
		require.NoError(t, ValidateDescriptor(d))
	})

	t.Run("action requires success", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityAction}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityAction: {Type: "object", Properties: map[string]*jsonschema.Schema{}},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires output field")
	})

	t.Run("authentication requires fields", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityAuthentication}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityAuthentication: {Type: "object", Properties: map[string]*jsonschema.Schema{}},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires output field")
	})

	t.Run("wrong field type", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityAction}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityAction: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"success": {Type: "string"},
				},
			},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be type")
	})

	t.Run("nil schema in output schemas", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityValidation}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityValidation: nil,
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires output field")
	})

	t.Run("nil property in schema properties", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityAction}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityAction: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"success": nil,
				},
			},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires output field")
	})

	t.Run("from and transform need no required fields", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{CapabilityFrom, CapabilityTransform}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			CapabilityFrom:      {Type: "object"},
			CapabilityTransform: {Type: "object"},
		}
		require.NoError(t, ValidateDescriptor(d))
	})

	t.Run("unknown capability rejected", func(t *testing.T) {
		d := validDescriptor()
		d.Capabilities = []Capability{"unknown-cap"}
		d.OutputSchemas = map[Capability]*jsonschema.Schema{
			"unknown-cap": {Type: "object"},
		}
		err := ValidateDescriptor(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown capability "unknown-cap"`)
	})
}
