// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
)

// ---- Descriptor conversion benchmarks ----

func newBenchDescriptor() *provider.Descriptor {
	min5 := 5
	max100 := 100
	minVal := 0.0
	maxVal := 100.0
	return &provider.Descriptor{
		Name: "bench-provider", DisplayName: "Bench Provider", Description: "A benchmark provider",
		APIVersion: "v1", Version: semver.MustParse("2.0.0"),
		Category: "utility", Tags: []string{"bench", "test", "perf"},
		Capabilities:    []provider.Capability{provider.CapabilityTransform, provider.CapabilityFrom},
		SensitiveFields: []string{"password", "token"},
		Links: []provider.Link{
			{Name: "docs", URL: "https://example.com/docs"},
			{Name: "source", URL: "https://github.com/example"},
		},
		Examples: []provider.Example{
			{Name: "basic", Description: "Basic usage", YAML: "message: hello"},
			{Name: "advanced", Description: "Advanced usage", YAML: "message: hello\nuppercase: true"},
		},
		Maintainers: []provider.Contact{
			{Name: "Alice", Email: "alice@example.com"},
			{Name: "Bob", Email: "bob@example.com"},
		},
		Schema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{"message", "count"},
			Properties: map[string]*jsonschema.Schema{
				"message": {
					Type: "string", Description: "The message",
					MinLength: &min5, MaxLength: &max100,
					Pattern:  "^[a-zA-Z]+$",
					Examples: []any{"hello"},
				},
				"count": {
					Type: "integer", Description: "Repeat count",
					Minimum: &minVal, Maximum: &maxVal,
				},
				"tags": {
					Type: "array", Description: "Tags",
					Enum: []any{"a", "b", "c"},
				},
			},
		},
		OutputSchemas: map[provider.Capability]*jsonschema.Schema{
			provider.CapabilityTransform: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"result": {Type: "string", Description: "The result"},
				},
			},
		},
	}
}

func BenchmarkDescriptorToProto(b *testing.B) {
	desc := newBenchDescriptor()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = descriptorToProto(desc)
	}
}

func BenchmarkSchemaToProto(b *testing.B) {
	desc := newBenchDescriptor()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = schemaToProto(desc.Schema)
	}
}

func BenchmarkParamToProto(b *testing.B) {
	desc := newBenchDescriptor()
	prop := desc.Schema.Properties["message"]
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = paramToProto(prop, true)
	}
}

// ---- Context application benchmarks ----

func BenchmarkApplyRequestContext(b *testing.B) {
	contextJSON, _ := json.Marshal(map[string]any{
		"resolverContext": map[string]any{"host": "example.com", "port": 8080},
	})
	paramsJSON, _ := json.Marshal(map[string]any{"env": "prod", "region": "us-east-1"})
	iterJSON, _ := json.Marshal("item-value")

	req := &proto.ExecuteProviderRequest{
		DryRun: true, ExecutionMode: "transform",
		WorkingDirectory: "/work", OutputDirectory: "/out",
		ConflictStrategy: "overwrite", Backup: true,
		Context: contextJSON, Parameters: paramsJSON,
		IterationContext: &proto.IterationContext{
			Item: iterJSON, Index: 5, ItemAlias: "srv", IndexAlias: "idx",
		},
		SolutionMetadata: &proto.SolutionMeta{
			Name: "sol", Version: "1.0.0", DisplayName: "Solution",
			Description: "desc", Category: "infra", Tags: []string{"t1", "t2"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = applyRequestContext(context.Background(), req)
	}
}

func BenchmarkApplyRequestContext_Minimal(b *testing.B) {
	req := &proto.ExecuteProviderRequest{
		DryRun:        true,
		ExecutionMode: "transform",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = applyRequestContext(context.Background(), req)
	}
}

// ---- ExecuteProvider response marshaling benchmark ----

func BenchmarkExecuteProviderMarshal(b *testing.B) {
	output := &provider.Output{
		Data: map[string]any{
			"result": "hello world",
			"count":  42,
			"nested": map[string]any{"key": "value"},
		},
		Warnings: []string{"warning1", "warning2"},
		Metadata: map[string]any{"latency_ms": 15, "cached": true},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = json.Marshal(output)
	}
}

// ---- Stream forwarder benchmark ----

func BenchmarkStreamForwarder(b *testing.B) {
	stream := &benchMockStream{ctx: context.Background()}
	f := newStreamForwarder(stream)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f.forward(StreamChunk{Stdout: []byte("output line\n")})
	}
}

type benchMockStream struct {
	proto.PluginService_ExecuteProviderStreamServer
	ctx context.Context
}

func (s *benchMockStream) Context() context.Context { return s.ctx }

func (s *benchMockStream) Send(_ *proto.ExecuteProviderStreamChunk) error {
	return nil
}

// ---- GRPCServer RPC benchmarks ----

func BenchmarkGRPCServer_GetProviders(b *testing.B) {
	srv := &GRPCServer{Impl: &benchMockProvider{}}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = srv.GetProviders(context.Background(), &proto.GetProvidersRequest{})
	}
}

func BenchmarkGRPCServer_ExecuteProvider(b *testing.B) {
	srv := &GRPCServer{Impl: &benchMockProvider{}}
	inputJSON, _ := json.Marshal(map[string]any{"message": "hello"})
	req := &proto.ExecuteProviderRequest{ProviderName: "bench", Input: inputJSON}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = srv.ExecuteProvider(context.Background(), req)
	}
}

func BenchmarkGRPCServer_ConfigureProvider(b *testing.B) {
	srv := &GRPCServer{Impl: &benchMockProvider{}}
	req := &proto.ConfigureProviderRequest{
		ProviderName: "bench", BinaryName: "scafctl",
		Settings: map[string][]byte{"key": []byte(`"value"`)},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = srv.ConfigureProvider(context.Background(), req)
	}
}

// benchMockProvider is a minimal provider mock for benchmarks.
type benchMockProvider struct{}

func (m *benchMockProvider) GetProviders(_ context.Context) ([]string, error) {
	return []string{"bench"}, nil
}

func (m *benchMockProvider) GetProviderDescriptor(_ context.Context, _ string) (*provider.Descriptor, error) {
	return newBenchDescriptor(), nil
}

func (m *benchMockProvider) ConfigureProvider(_ context.Context, _ string, _ ProviderConfig) error {
	return nil
}

func (m *benchMockProvider) ExecuteProvider(_ context.Context, _ string, input map[string]any) (*provider.Output, error) {
	return &provider.Output{Data: input}, nil
}

func (m *benchMockProvider) ExecuteProviderStream(_ context.Context, _ string, _ map[string]any, _ func(StreamChunk)) error {
	return ErrStreamingNotSupported
}

func (m *benchMockProvider) DescribeWhatIf(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "would do thing", nil
}

func (m *benchMockProvider) ExtractDependencies(_ context.Context, _ string, _ map[string]any) ([]string, error) {
	return nil, nil
}

func (m *benchMockProvider) StopProvider(_ context.Context, _ string) error {
	return nil
}
