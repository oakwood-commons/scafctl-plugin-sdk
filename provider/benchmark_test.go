// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
)

// ---- Context benchmarks ----

func BenchmarkWithDryRun(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = WithDryRun(ctx, true)
	}
}

func BenchmarkDryRunFromContext(b *testing.B) {
	ctx := WithDryRun(context.Background(), true)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = DryRunFromContext(ctx)
	}
}

func BenchmarkContextRoundTrip_Full(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx := context.Background()
		ctx = WithExecutionMode(ctx, CapabilityTransform)
		ctx = WithDryRun(ctx, true)
		ctx = WithResolverContext(ctx, map[string]any{"host": "example.com"})
		ctx = WithParameters(ctx, map[string]any{"env": "prod"})
		ctx = WithWorkingDirectory(ctx, "/work")
		ctx = WithOutputDirectory(ctx, "/out")
		ctx = WithConflictStrategy(ctx, "overwrite")
		ctx = WithBackup(ctx, true)
		ctx = WithIterationContext(ctx, &IterationContext{
			Item: "item1", Index: 0, ItemAlias: "srv", IndexAlias: "idx",
		})
		ctx = WithSolutionMetadata(ctx, &SolutionMeta{
			Name: "sol", Version: "1.0.0", DisplayName: "Solution",
		})

		_, _ = ExecutionModeFromContext(ctx)
		_ = DryRunFromContext(ctx)
		_, _ = ResolverContextFromContext(ctx)
		_, _ = ParametersFromContext(ctx)
		_, _ = WorkingDirectoryFromContext(ctx)
		_, _ = OutputDirectoryFromContext(ctx)
		_, _ = ConflictStrategyFromContext(ctx)
		_, _ = BackupFromContext(ctx)
		_, _ = IterationContextFromContext(ctx)
		_, _ = SolutionMetadataFromContext(ctx)
	}
}

// ---- Capability benchmarks ----

func BenchmarkCapability_IsValid(b *testing.B) {
	caps := []Capability{CapabilityFrom, CapabilityTransform, CapabilityValidation, CapabilityAuthentication, CapabilityAction, "invalid"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for _, c := range caps {
			_ = c.IsValid()
		}
	}
}

// ---- Descriptor benchmarks ----

func BenchmarkDescriptor_IsSensitiveField(b *testing.B) {
	d := &Descriptor{
		SensitiveFields: []string{"password", "token", "secret", "apiKey", "private_key"},
	}

	b.Run("Hit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = d.IsSensitiveField("secret")
		}
	})

	b.Run("Miss", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = d.IsSensitiveField("username")
		}
	})
}

func BenchmarkDescriptor_DescribeWhatIf(b *testing.B) {
	b.Run("WithFunc", func(b *testing.B) {
		d := &Descriptor{
			Name: "test",
			WhatIf: func(_ context.Context, _ any) (string, error) {
				return "Would do something", nil
			},
		}
		b.ReportAllocs()
		for b.Loop() {
			_ = d.DescribeWhatIf(context.Background(), nil)
		}
	})

	b.Run("Fallback", func(b *testing.B) {
		d := &Descriptor{Name: "test"}
		b.ReportAllocs()
		for b.Loop() {
			_ = d.DescribeWhatIf(context.Background(), nil)
		}
	})
}

// ---- Validation benchmarks ----

func BenchmarkSchemaValidator_ValidateInputs(b *testing.B) {
	sv := NewSchemaValidator()
	min1 := 1
	max100 := 100
	schema := &jsonschema.Schema{
		Type:     "object",
		Required: []string{"name"},
		Properties: map[string]*jsonschema.Schema{
			"name":  {Type: "string", Description: "Name", MinLength: &min1, MaxLength: &max100},
			"count": {Type: "integer", Description: "Count"},
		},
	}
	inputs := map[string]any{"name": "hello", "count": 42}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = sv.ValidateInputs(inputs, schema)
	}
}

func BenchmarkValidationErrors_Error(b *testing.B) {
	errs := ValidationErrors{
		{Field: "name", Message: "required"},
		{Field: "count", Message: "must be positive", Actual: "-1", Expected: ">0"},
		{Field: "tags", Message: "too many items"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = errs.Error()
	}
}

// ---- Descriptor validation benchmark ----

func BenchmarkDescriptor_Helpers(b *testing.B) {
	desc := &Descriptor{
		Name: "test-provider", DisplayName: "Test Provider", Description: "A test provider for benchmarks",
		APIVersion: "v1", Version: semver.MustParse("1.0.0"),
		Capabilities: []Capability{CapabilityTransform},
		Schema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"input"},
			Properties: map[string]*jsonschema.Schema{
				"input": {Type: "string"},
			},
		},
		OutputSchemas: map[Capability]*jsonschema.Schema{
			CapabilityTransform: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"result": {Type: "string"},
				},
			},
		},
	}
	_ = desc

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = desc.IsSensitiveField("password")
		_ = desc.DescribeWhatIf(context.Background(), nil)
	}
}
