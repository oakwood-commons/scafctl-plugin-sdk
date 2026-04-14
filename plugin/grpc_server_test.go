// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- mock impl for ProviderPlugin ---

type mockProvider struct {
	getProviders          func(ctx context.Context) ([]string, error)
	getProviderDescriptor func(ctx context.Context, name string) (*provider.Descriptor, error)
	configureProvider     func(ctx context.Context, name string, cfg ProviderConfig) error
	executeProvider       func(ctx context.Context, name string, input map[string]any) (*provider.Output, error)
	executeProviderStream func(ctx context.Context, name string, input map[string]any, cb func(StreamChunk)) error
	describeWhatIf        func(ctx context.Context, name string, input map[string]any) (string, error)
	extractDependencies   func(ctx context.Context, name string, inputs map[string]any) ([]string, error)
	stopProvider          func(ctx context.Context, name string) error
}

//nolint:dupl // mock mirrors ProviderPlugin interface
func (m *mockProvider) GetProviders(ctx context.Context) ([]string, error) {
	if m.getProviders != nil {
		return m.getProviders(ctx)
	}
	return nil, nil
}

func (m *mockProvider) GetProviderDescriptor(ctx context.Context, name string) (*provider.Descriptor, error) {
	if m.getProviderDescriptor != nil {
		return m.getProviderDescriptor(ctx, name)
	}
	return nil, errors.New("not found")
}

func (m *mockProvider) ConfigureProvider(ctx context.Context, name string, cfg ProviderConfig) error {
	if m.configureProvider != nil {
		return m.configureProvider(ctx, name, cfg)
	}
	return nil
}

func (m *mockProvider) ExecuteProvider(ctx context.Context, name string, input map[string]any) (*provider.Output, error) {
	if m.executeProvider != nil {
		return m.executeProvider(ctx, name, input)
	}
	return nil, errors.New("not implemented")
}

func (m *mockProvider) ExecuteProviderStream(ctx context.Context, name string, input map[string]any, cb func(StreamChunk)) error {
	if m.executeProviderStream != nil {
		return m.executeProviderStream(ctx, name, input, cb)
	}
	return ErrStreamingNotSupported
}

func (m *mockProvider) DescribeWhatIf(ctx context.Context, name string, input map[string]any) (string, error) {
	if m.describeWhatIf != nil {
		return m.describeWhatIf(ctx, name, input)
	}
	return "", nil
}

func (m *mockProvider) ExtractDependencies(ctx context.Context, name string, inputs map[string]any) ([]string, error) {
	if m.extractDependencies != nil {
		return m.extractDependencies(ctx, name, inputs)
	}
	return nil, nil
}

func (m *mockProvider) StopProvider(ctx context.Context, name string) error {
	if m.stopProvider != nil {
		return m.stopProvider(ctx, name)
	}
	return nil
}

// --- mock stream ---

type mockStream struct {
	proto.PluginService_ExecuteProviderStreamServer
	ctx    context.Context
	chunks []*proto.ExecuteProviderStreamChunk
}

func (s *mockStream) Context() context.Context { return s.ctx }
func (s *mockStream) Send(chunk *proto.ExecuteProviderStreamChunk) error {
	s.chunks = append(s.chunks, chunk)
	return nil
}

// --- GRPCServer Tests ---

func TestGRPCServer_GetProviders(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		getProviders: func(_ context.Context) ([]string, error) {
			return []string{"echo", "http"}, nil
		},
	}}
	resp, err := srv.GetProviders(context.Background(), &proto.GetProvidersRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"echo", "http"}, resp.ProviderNames)
}

func TestGRPCServer_GetProviders_Error(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		getProviders: func(_ context.Context) ([]string, error) {
			return nil, errors.New("oops")
		},
	}}
	_, err := srv.GetProviders(context.Background(), &proto.GetProvidersRequest{})
	require.Error(t, err)
}

func TestGRPCServer_GetProviderDescriptor(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		getProviderDescriptor: func(_ context.Context, name string) (*provider.Descriptor, error) {
			return &provider.Descriptor{
				Name: name, DisplayName: "Test", Description: "desc",
				APIVersion: "v1", Version: semver.MustParse("1.0.0"),
				Capabilities: []provider.Capability{provider.CapabilityTransform},
				Schema:       &jsonschema.Schema{Type: "object"},
			}, nil
		},
	}}
	resp, err := srv.GetProviderDescriptor(context.Background(), &proto.GetProviderDescriptorRequest{ProviderName: "test"})
	require.NoError(t, err)
	assert.Equal(t, "test", resp.Descriptor_.Name)
	assert.Equal(t, "1.0.0", resp.Descriptor_.Version)
}

func TestGRPCServer_ExecuteProvider(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProvider: func(_ context.Context, name string, input map[string]any) (*provider.Output, error) {
			return &provider.Output{Data: map[string]any{"echo": input["msg"]}}, nil
		},
	}}
	inputJSON, _ := json.Marshal(map[string]any{"msg": "hello"})
	resp, err := srv.ExecuteProvider(context.Background(), &proto.ExecuteProviderRequest{
		ProviderName: "test", Input: inputJSON,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	assert.NotEmpty(t, resp.Output)
}

func TestGRPCServer_ExecuteProvider_InvalidInput(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	resp, err := srv.ExecuteProvider(context.Background(), &proto.ExecuteProviderRequest{
		ProviderName: "test", Input: []byte("not-json{{{"),
	})
	require.NoError(t, err) // errors returned in response, not gRPC error
	assert.Contains(t, resp.Error, "failed to decode input")
}

func TestGRPCServer_ExecuteProvider_ImplError(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProvider: func(_ context.Context, _ string, _ map[string]any) (*provider.Output, error) {
			return nil, errors.New("exec failed")
		},
	}}
	inputJSON, _ := json.Marshal(map[string]any{})
	resp, err := srv.ExecuteProvider(context.Background(), &proto.ExecuteProviderRequest{
		ProviderName: "test", Input: inputJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, "exec failed", resp.Error)
}

func TestGRPCServer_ConfigureProvider(t *testing.T) {
	var gotCfg ProviderConfig
	srv := &GRPCServer{Impl: &mockProvider{
		configureProvider: func(_ context.Context, _ string, cfg ProviderConfig) error {
			gotCfg = cfg
			return nil
		},
	}}
	resp, err := srv.ConfigureProvider(context.Background(), &proto.ConfigureProviderRequest{
		ProviderName: "test", Quiet: true, NoColor: true, BinaryName: "bin",
		HostServiceId: 42, Settings: map[string][]byte{"k": []byte(`"v"`)},
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
	assert.Equal(t, PluginProtocolVersion, resp.ProtocolVersion)
	assert.True(t, gotCfg.Quiet)
	assert.True(t, gotCfg.NoColor)
	assert.Equal(t, "bin", gotCfg.BinaryName)
	assert.Equal(t, uint32(42), gotCfg.HostServiceID)
}

func TestGRPCServer_ConfigureProvider_Error(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		configureProvider: func(_ context.Context, _ string, _ ProviderConfig) error {
			return errors.New("config error")
		},
	}}
	resp, err := srv.ConfigureProvider(context.Background(), &proto.ConfigureProviderRequest{ProviderName: "test"})
	require.NoError(t, err)
	assert.Equal(t, "config error", resp.Error)
}

func TestGRPCServer_DescribeWhatIf(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		describeWhatIf: func(_ context.Context, _ string, input map[string]any) (string, error) {
			return "would do thing", nil
		},
	}}
	inputJSON, _ := json.Marshal(map[string]any{"x": 1})
	resp, err := srv.DescribeWhatIf(context.Background(), &proto.DescribeWhatIfRequest{
		ProviderName: "test", Input: inputJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, "would do thing", resp.Description)
}

func TestGRPCServer_DescribeWhatIf_InvalidInput(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	resp, err := srv.DescribeWhatIf(context.Background(), &proto.DescribeWhatIfRequest{
		ProviderName: "test", Input: []byte("bad{"),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "failed to decode input")
}

func TestGRPCServer_DescribeWhatIf_ImplError(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		describeWhatIf: func(_ context.Context, _ string, _ map[string]any) (string, error) {
			return "", errors.New("fail")
		},
	}}
	resp, err := srv.DescribeWhatIf(context.Background(), &proto.DescribeWhatIfRequest{ProviderName: "test"})
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Error)
}

func TestGRPCServer_ExtractDependencies(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		extractDependencies: func(_ context.Context, _ string, _ map[string]any) ([]string, error) {
			return []string{"dep1", "dep2"}, nil
		},
	}}
	inputJSON, _ := json.Marshal(map[string]any{"a": 1})
	resp, err := srv.ExtractDependencies(context.Background(), &proto.ExtractDependenciesRequest{
		ProviderName: "test", Inputs: inputJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"dep1", "dep2"}, resp.Dependencies)
}

func TestGRPCServer_ExtractDependencies_InvalidInput(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	resp, err := srv.ExtractDependencies(context.Background(), &proto.ExtractDependenciesRequest{
		ProviderName: "test", Inputs: []byte("{bad"),
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "failed to decode inputs")
}

func TestGRPCServer_StopProvider(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	resp, err := srv.StopProvider(context.Background(), &proto.StopProviderRequest{ProviderName: "test"})
	require.NoError(t, err)
	assert.Empty(t, resp.Error)
}

func TestGRPCServer_StopProvider_Error(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		stopProvider: func(_ context.Context, _ string) error {
			return errors.New("stop error")
		},
	}}
	resp, err := srv.StopProvider(context.Background(), &proto.StopProviderRequest{ProviderName: "test"})
	require.NoError(t, err)
	assert.Equal(t, "stop error", resp.Error)
}

// --- ExecuteProviderStream Tests ---

func TestGRPCServer_ExecuteProviderStream(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProviderStream: func(_ context.Context, _ string, input map[string]any, cb func(StreamChunk)) error {
			cb(StreamChunk{Stdout: []byte("hello")})
			cb(StreamChunk{Stderr: []byte("warn")})
			cb(StreamChunk{Result: &provider.Output{Data: map[string]any{"ok": true}}})
			return nil
		},
	}}
	stream := &mockStream{ctx: context.Background()}
	inputJSON, _ := json.Marshal(map[string]any{"x": 1})
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{
		ProviderName: "test", Input: inputJSON,
	}, stream)
	require.NoError(t, err)
	assert.Len(t, stream.chunks, 3)
	assert.NotNil(t, stream.chunks[0].GetStdout())
	assert.NotNil(t, stream.chunks[1].GetStderr())
	assert.NotNil(t, stream.chunks[2].GetResult())
}

func TestGRPCServer_ExecuteProviderStream_Unimplemented(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	stream := &mockStream{ctx: context.Background()}
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{ProviderName: "test"}, stream)
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unimplemented, st.Code())
}

func TestGRPCServer_ExecuteProviderStream_InvalidInput(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{}}
	stream := &mockStream{ctx: context.Background()}
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{
		ProviderName: "test", Input: []byte("bad{"),
	}, stream)
	require.NoError(t, err)
	require.Len(t, stream.chunks, 1)
	assert.Contains(t, stream.chunks[0].GetResult().Error, "failed to decode input")
}

func TestGRPCServer_ExecuteProviderStream_ErrorChunk(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProviderStream: func(_ context.Context, _ string, _ map[string]any, cb func(StreamChunk)) error {
			cb(StreamChunk{Error: "some error"})
			return nil
		},
	}}
	stream := &mockStream{ctx: context.Background()}
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{ProviderName: "test"}, stream)
	require.NoError(t, err)
	require.Len(t, stream.chunks, 1)
	assert.Equal(t, "some error", stream.chunks[0].GetResult().Error)
}

func TestGRPCServer_ExecuteProviderStream_EmptyChunk(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProviderStream: func(_ context.Context, _ string, _ map[string]any, cb func(StreamChunk)) error {
			cb(StreamChunk{}) // empty chunk should be ignored
			return nil
		},
	}}
	stream := &mockStream{ctx: context.Background()}
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{ProviderName: "test"}, stream)
	require.NoError(t, err)
	assert.Empty(t, stream.chunks)
}

func TestGRPCServer_ExecuteProviderStream_NonStreamingError(t *testing.T) {
	srv := &GRPCServer{Impl: &mockProvider{
		executeProviderStream: func(_ context.Context, _ string, _ map[string]any, _ func(StreamChunk)) error {
			return errors.New("some other error")
		},
	}}
	stream := &mockStream{ctx: context.Background()}
	err := srv.ExecuteProviderStream(&proto.ExecuteProviderRequest{ProviderName: "test"}, stream)
	require.NoError(t, err)
	require.Len(t, stream.chunks, 1)
	assert.Equal(t, "some other error", stream.chunks[0].GetResult().Error)
}

// --- Context helper tests ---

func TestApplyRequestContext(t *testing.T) {
	ctx := context.Background()
	contextJSON, _ := json.Marshal(map[string]any{"resolverContext": map[string]any{"key": "val"}})
	iterJSON, _ := json.Marshal("item1")
	paramsJSON, _ := json.Marshal(map[string]any{"p1": "v1"})

	req := &proto.ExecuteProviderRequest{
		DryRun: true, ExecutionMode: "transform",
		WorkingDirectory: "/work", OutputDirectory: "/out",
		ConflictStrategy: "overwrite", Backup: true,
		Context: contextJSON, Parameters: paramsJSON,
		IterationContext: &proto.IterationContext{
			Item: iterJSON, Index: 2, ItemAlias: "srv", IndexAlias: "idx",
		},
		SolutionMetadata: &proto.SolutionMeta{
			Name: "sol", Version: "2.0.0", DisplayName: "Sol",
			Description: "desc", Category: "infra", Tags: []string{"t1"},
		},
	}

	ctx, err := applyRequestContext(ctx, req)
	require.NoError(t, err)

	assert.True(t, provider.DryRunFromContext(ctx))
	mode, ok := provider.ExecutionModeFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, provider.CapabilityTransform, mode)

	wd, ok := provider.WorkingDirectoryFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "/work", wd)

	od, ok := provider.OutputDirectoryFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "/out", od)

	cs, ok := provider.ConflictStrategyFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "overwrite", cs)

	b, ok := provider.BackupFromContext(ctx)
	assert.True(t, ok)
	assert.True(t, b)

	rc, ok := provider.ResolverContextFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "val", rc["key"])

	params, ok := provider.ParametersFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "v1", params["p1"])

	iter, ok := provider.IterationContextFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "item1", iter.Item)
	assert.Equal(t, 2, iter.Index)
	assert.Equal(t, "srv", iter.ItemAlias)

	meta, ok := provider.SolutionMetadataFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "sol", meta.Name)
}

func TestApplyRequestContext_InvalidContext(t *testing.T) {
	_, err := applyRequestContext(context.Background(), &proto.ExecuteProviderRequest{
		Context: []byte("{bad"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode context")
}

func TestApplyRequestContext_InvalidParams(t *testing.T) {
	_, err := applyRequestContext(context.Background(), &proto.ExecuteProviderRequest{
		Parameters: []byte("{bad"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode parameters")
}

func TestApplyRequestContext_InvalidIterationItem(t *testing.T) {
	_, err := applyRequestContext(context.Background(), &proto.ExecuteProviderRequest{
		IterationContext: &proto.IterationContext{Item: []byte("{bad")},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode iteration item")
}

func TestUnmarshalSolutionMeta_Nil(t *testing.T) {
	ctx := unmarshalSolutionMeta(context.Background(), nil)
	_, ok := provider.SolutionMetadataFromContext(ctx)
	assert.False(t, ok)
}

func TestUnmarshalIterationContext_Nil(t *testing.T) {
	ctx, err := unmarshalIterationContext(context.Background(), nil)
	require.NoError(t, err)
	_, ok := provider.IterationContextFromContext(ctx)
	assert.False(t, ok)
}

// --- Descriptor conversion round-trip ---

func TestDescriptorRoundTrip(t *testing.T) {
	min5 := 5
	max100 := 100
	min0 := 0.0
	max99 := 99.0
	exMin := 0.0
	exMax := 100.0
	minItems := 1
	maxItems := 10
	desc := &provider.Descriptor{
		Name: "test", DisplayName: "Test Provider", Description: "A test",
		APIVersion: "v1", Version: semver.MustParse("2.3.4"),
		Category: "utility", Tags: []string{"tag1", "tag2"},
		Icon: "https://example.com/icon.png", IsDeprecated: true, Beta: true,
		Capabilities:    []provider.Capability{provider.CapabilityTransform, provider.CapabilityFrom},
		SensitiveFields: []string{"secret"},
		Links:           []provider.Link{{Name: "docs", URL: "https://example.com"}},
		Examples:        []provider.Example{{Name: "ex1", Description: "desc", YAML: "yaml: true"}},
		Maintainers:     []provider.Contact{{Name: "Jane", Email: "jane@ex.com"}},
		Schema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"name": {
					Type: "string", Description: "the name",
					MinLength: &min5, MaxLength: &max100, Pattern: "^[a-z]+$", Format: "hostname",
					Default: json.RawMessage(`"default"`), Examples: []any{"example"},
				},
				"age": {
					Type:    "integer",
					Minimum: &min0, Maximum: &max99,
					ExclusiveMinimum: &exMin, ExclusiveMaximum: &exMax,
				},
				"items": {
					Type:     "array",
					MinItems: &minItems, MaxItems: &maxItems,
				},
				"choice": {
					Type: "string",
					Enum: []any{"a", "b"},
				},
			},
			Required: []string{"name"},
		},
		OutputSchemas: map[provider.Capability]*jsonschema.Schema{
			provider.CapabilityTransform: {
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"result": {Type: "string"},
				},
			},
		},
		ExtractDependencies: func(_ map[string]any) []string { return nil },
	}

	pd := descriptorToProto(desc)
	assert.Equal(t, "test", pd.Name)
	assert.Equal(t, "2.3.4", pd.Version)
	assert.True(t, pd.HasExtractDependencies)
	assert.NotNil(t, pd.Schema)
	assert.NotNil(t, pd.RawSchema)

	roundTripped, err := ProtoToDescriptor(pd)
	require.NoError(t, err)
	assert.Equal(t, desc.Name, roundTripped.Name)
	assert.Equal(t, desc.DisplayName, roundTripped.DisplayName)
	assert.Equal(t, desc.Version.String(), roundTripped.Version.String())
	assert.Equal(t, desc.Category, roundTripped.Category)
	assert.True(t, roundTripped.IsDeprecated)
	assert.True(t, roundTripped.Beta)
	assert.Equal(t, len(desc.Capabilities), len(roundTripped.Capabilities))
	assert.Equal(t, len(desc.Links), len(roundTripped.Links))
	assert.Equal(t, len(desc.Examples), len(roundTripped.Examples))
	assert.Equal(t, len(desc.Maintainers), len(roundTripped.Maintainers))
	assert.NotNil(t, roundTripped.Schema)
	assert.NotNil(t, roundTripped.ExtractDependencies)
}

func TestProtoToDescriptor_InvalidVersion(t *testing.T) {
	_, err := ProtoToDescriptor(&proto.ProviderDescriptor{Name: "test", Version: "not-semver"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid semver version")
}

func TestProtoToDescriptor_NoVersion(t *testing.T) {
	desc, err := ProtoToDescriptor(&proto.ProviderDescriptor{Name: "test"})
	require.NoError(t, err)
	assert.Nil(t, desc.Version)
}

func TestDescriptorToProto_NilVersion(t *testing.T) {
	pd := descriptorToProto(&provider.Descriptor{Name: "test"})
	assert.Empty(t, pd.Version)
}

func TestDescriptorToProto_NilOutputSchemas(t *testing.T) {
	pd := descriptorToProto(&provider.Descriptor{Name: "test", OutputSchemas: map[provider.Capability]*jsonschema.Schema{
		provider.CapabilityFrom: nil,
	}})
	assert.Empty(t, pd.OutputSchemas)
}

// --- Schema conversion ---

func TestSchemaToProto_Nil(t *testing.T) {
	assert.Nil(t, schemaToProto(nil))
}

func TestSchemaToProto_NoProperties(t *testing.T) {
	assert.Nil(t, schemaToProto(&jsonschema.Schema{Type: "object"}))
}

func TestProtoSchemaToJSON(t *testing.T) {
	ps := &proto.Schema{
		Parameters: map[string]*proto.Parameter{
			"name": {Type: "string", Required: true, Description: "the name"},
			"age":  {Type: "integer", Required: false},
		},
	}
	schema := protoSchemaToJSON(ps)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "name")
	assert.Contains(t, schema.Properties, "age")
	assert.Contains(t, schema.Required, "name")
}

func TestProtoSchemaToJSON_RequiredIsSorted(t *testing.T) {
	ps := &proto.Schema{
		Parameters: map[string]*proto.Parameter{
			"zebra":  {Type: "string", Required: true},
			"alpha":  {Type: "string", Required: true},
			"middle": {Type: "string", Required: true},
		},
	}
	schema := protoSchemaToJSON(ps)
	assert.Equal(t, []string{"alpha", "middle", "zebra"}, schema.Required)
}

func TestUnmarshalSchemaOrFallback(t *testing.T) {
	t.Run("raw JSON wins", func(t *testing.T) {
		raw, _ := json.Marshal(&jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{
			"x": {Type: "string"},
		}})
		schema := unmarshalSchemaOrFallback(raw, nil)
		require.NotNil(t, schema)
		assert.Equal(t, "object", schema.Type)
	})

	t.Run("fallback to proto schema", func(t *testing.T) {
		ps := &proto.Schema{Parameters: map[string]*proto.Parameter{"y": {Type: "integer"}}}
		schema := unmarshalSchemaOrFallback(nil, ps)
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "y")
	})

	t.Run("both nil", func(t *testing.T) {
		assert.Nil(t, unmarshalSchemaOrFallback(nil, nil))
	})

	t.Run("invalid raw falls back to proto", func(t *testing.T) {
		ps := &proto.Schema{Parameters: map[string]*proto.Parameter{"z": {Type: "string"}}}
		schema := unmarshalSchemaOrFallback([]byte("invalid{"), ps)
		require.NotNil(t, schema)
		assert.Contains(t, schema.Properties, "z")
	})
}

func TestUnmarshalOutputSchemas(t *testing.T) {
	t.Run("raw schemas", func(t *testing.T) {
		raw := map[string][]byte{}
		schemaBytes, _ := json.Marshal(&jsonschema.Schema{Type: "object"})
		raw["transform"] = schemaBytes
		out := unmarshalOutputSchemas(raw, nil)
		assert.Contains(t, out, provider.Capability("transform"))
	})

	t.Run("proto schemas fallback", func(t *testing.T) {
		protoSchemas := map[string]*proto.Schema{
			"from": {Parameters: map[string]*proto.Parameter{"x": {Type: "string"}}},
		}
		out := unmarshalOutputSchemas(nil, protoSchemas)
		assert.Contains(t, out, provider.Capability("from"))
	})

	t.Run("both nil", func(t *testing.T) {
		assert.Nil(t, unmarshalOutputSchemas(nil, nil))
	})

	t.Run("nil proto schema entry skipped", func(t *testing.T) {
		protoSchemas := map[string]*proto.Schema{"from": nil}
		out := unmarshalOutputSchemas(nil, protoSchemas)
		assert.Empty(t, out)
	})
}

func TestProtoParamToJSON_AllFields(t *testing.T) {
	p := &proto.Parameter{
		Type: "string", Description: "desc", Pattern: "^a$", Format: "uri",
		DefaultValue: []byte(`"def"`), Example: `"ex"`,
		MaxLength: 100, MinLength: 1,
		HasMinimum: true, Minimum: 0, HasMaximum: true, Maximum: 99,
		HasExclusiveMinimum: true, ExclusiveMinimum: -1,
		HasExclusiveMaximum: true, ExclusiveMaximum: 100,
		MinItems: 1, MaxItems: 10,
		EnumValues: [][]byte{[]byte(`"a"`), []byte(`"b"`)},
	}
	schema := protoParamToJSON(p)
	assert.Equal(t, "string", schema.Type)
	assert.Equal(t, "desc", schema.Description)
	assert.NotNil(t, schema.MaxLength)
	assert.NotNil(t, schema.MinLength)
	assert.NotNil(t, schema.Minimum)
	assert.NotNil(t, schema.Maximum)
	assert.NotNil(t, schema.ExclusiveMinimum)
	assert.NotNil(t, schema.ExclusiveMaximum)
	assert.NotNil(t, schema.MinItems)
	assert.NotNil(t, schema.MaxItems)
	assert.Len(t, schema.Enum, 2)
	assert.NotNil(t, schema.Default)
	assert.Len(t, schema.Examples, 1)
}
