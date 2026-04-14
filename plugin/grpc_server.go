// Copyright 2025-2026 Oakwood Commons
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/google/jsonschema-go/jsonschema"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/oakwood-commons/scafctl-plugin-sdk/plugin/proto"
	"github.com/oakwood-commons/scafctl-plugin-sdk/provider"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCServer implements the gRPC server for the plugin.
type GRPCServer struct {
	proto.UnimplementedPluginServiceServer
	Impl   ProviderPlugin
	broker *goplugin.GRPCBroker
}

//nolint:revive // req is required by gRPC interface
func (s *GRPCServer) GetProviders(ctx context.Context, _ *proto.GetProvidersRequest) (*proto.GetProvidersResponse, error) {
	providers, err := s.Impl.GetProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetProviders: %w", err)
	}
	return &proto.GetProvidersResponse{ProviderNames: providers}, nil
}

func (s *GRPCServer) GetProviderDescriptor(ctx context.Context, req *proto.GetProviderDescriptorRequest) (*proto.GetProviderDescriptorResponse, error) {
	desc, err := s.Impl.GetProviderDescriptor(ctx, req.ProviderName)
	if err != nil {
		return nil, fmt.Errorf("GetProviderDescriptor %q: %w", req.ProviderName, err)
	}
	return &proto.GetProviderDescriptorResponse{Descriptor_: descriptorToProto(desc)}, nil
}

func (s *GRPCServer) ExecuteProvider(ctx context.Context, req *proto.ExecuteProviderRequest) (*proto.ExecuteProviderResponse, error) {
	var input map[string]any
	if len(req.Input) > 0 {
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return &proto.ExecuteProviderResponse{Error: fmt.Sprintf("failed to decode input: %v", err)}, nil //nolint:nilerr
		}
	}
	ctx, ctxErr := applyRequestContext(ctx, req)
	if ctxErr != nil {
		return &proto.ExecuteProviderResponse{Error: ctxErr.Error()}, nil //nolint:nilerr
	}
	output, err := s.Impl.ExecuteProvider(ctx, req.ProviderName, input)
	if err != nil {
		return &proto.ExecuteProviderResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	outputBytes, err := json.Marshal(output)
	if err != nil {
		return &proto.ExecuteProviderResponse{Error: fmt.Sprintf("failed to encode output: %v", err)}, nil //nolint:nilerr
	}
	return &proto.ExecuteProviderResponse{Output: outputBytes}, nil
}

func (s *GRPCServer) ConfigureProvider(ctx context.Context, req *proto.ConfigureProviderRequest) (*proto.ConfigureProviderResponse, error) {
	settings := make(map[string]json.RawMessage, len(req.Settings))
	for k, v := range req.Settings {
		settings[k] = json.RawMessage(v)
	}
	cfg := ProviderConfig{
		Quiet:         req.Quiet,
		NoColor:       req.NoColor,
		BinaryName:    req.BinaryName,
		HostServiceID: req.HostServiceId,
		Settings:      settings,
	}
	if err := s.Impl.ConfigureProvider(ctx, req.ProviderName, cfg); err != nil {
		return &proto.ConfigureProviderResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.ConfigureProviderResponse{ProtocolVersion: PluginProtocolVersion}, nil
}

func (s *GRPCServer) ExecuteProviderStream(req *proto.ExecuteProviderRequest, stream proto.PluginService_ExecuteProviderStreamServer) error {
	ctx := stream.Context()
	var input map[string]any
	if len(req.Input) > 0 {
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return stream.Send(&proto.ExecuteProviderStreamChunk{
				Chunk: &proto.ExecuteProviderStreamChunk_Result{
					Result: &proto.ExecuteProviderResponse{Error: fmt.Sprintf("failed to decode input: %v", err)},
				},
			})
		}
	}
	ctx, ctxErr := applyRequestContext(ctx, req)
	if ctxErr != nil {
		return stream.Send(&proto.ExecuteProviderStreamChunk{
			Chunk: &proto.ExecuteProviderStreamChunk_Result{
				Result: &proto.ExecuteProviderResponse{Error: ctxErr.Error()},
			},
		})
	}
	forwarder := newStreamForwarder(stream)
	err := s.Impl.ExecuteProviderStream(ctx, req.ProviderName, input, forwarder.forward)
	if err != nil {
		if errors.Is(err, ErrStreamingNotSupported) {
			return status.Error(codes.Unimplemented, err.Error())
		}
		return stream.Send(&proto.ExecuteProviderStreamChunk{
			Chunk: &proto.ExecuteProviderStreamChunk_Result{
				Result: &proto.ExecuteProviderResponse{Error: err.Error()},
			},
		})
	}
	return nil
}

func (s *GRPCServer) DescribeWhatIf(ctx context.Context, req *proto.DescribeWhatIfRequest) (*proto.DescribeWhatIfResponse, error) {
	var input map[string]any
	if len(req.Input) > 0 {
		if err := json.Unmarshal(req.Input, &input); err != nil {
			return &proto.DescribeWhatIfResponse{Error: fmt.Sprintf("failed to decode input: %v", err)}, nil //nolint:nilerr
		}
	}
	description, err := s.Impl.DescribeWhatIf(ctx, req.ProviderName, input)
	if err != nil {
		return &proto.DescribeWhatIfResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.DescribeWhatIfResponse{Description: description}, nil
}

func (s *GRPCServer) ExtractDependencies(ctx context.Context, req *proto.ExtractDependenciesRequest) (*proto.ExtractDependenciesResponse, error) {
	var inputs map[string]any
	if len(req.Inputs) > 0 {
		if err := json.Unmarshal(req.Inputs, &inputs); err != nil {
			return &proto.ExtractDependenciesResponse{Error: fmt.Sprintf("failed to decode inputs: %v", err)}, nil //nolint:nilerr
		}
	}
	deps, err := s.Impl.ExtractDependencies(ctx, req.ProviderName, inputs)
	if err != nil {
		return &proto.ExtractDependenciesResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.ExtractDependenciesResponse{Dependencies: deps}, nil
}

func (s *GRPCServer) StopProvider(ctx context.Context, req *proto.StopProviderRequest) (*proto.StopProviderResponse, error) {
	if err := s.Impl.StopProvider(ctx, req.ProviderName); err != nil {
		return &proto.StopProviderResponse{Error: err.Error()}, nil //nolint:nilerr
	}
	return &proto.StopProviderResponse{}, nil
}

// ---- Context helpers ----

func unmarshalIterationContext(ctx context.Context, iter *proto.IterationContext) (context.Context, error) {
	if iter == nil {
		return ctx, nil
	}
	var item any
	if len(iter.Item) > 0 {
		if err := json.Unmarshal(iter.Item, &item); err != nil {
			return ctx, fmt.Errorf("failed to decode iteration item: %w", err)
		}
	}
	return provider.WithIterationContext(ctx, &provider.IterationContext{
		Item: item, Index: int(iter.Index), ItemAlias: iter.ItemAlias, IndexAlias: iter.IndexAlias,
	}), nil
}

func unmarshalSolutionMeta(ctx context.Context, meta *proto.SolutionMeta) context.Context {
	if meta == nil {
		return ctx
	}
	return provider.WithSolutionMetadata(ctx, &provider.SolutionMeta{
		Name: meta.Name, Version: meta.Version, DisplayName: meta.DisplayName,
		Description: meta.Description, Category: meta.Category, Tags: meta.Tags,
	})
}

func applyRequestContext(ctx context.Context, req *proto.ExecuteProviderRequest) (context.Context, error) {
	if len(req.Context) > 0 {
		var contextData map[string]any
		if err := json.Unmarshal(req.Context, &contextData); err != nil {
			return ctx, fmt.Errorf("failed to decode context: %w", err)
		}
		if resolverCtx, ok := contextData["resolverContext"].(map[string]any); ok {
			ctx = provider.WithResolverContext(ctx, resolverCtx)
		}
	}
	ctx = provider.WithDryRun(ctx, req.DryRun)
	if req.ExecutionMode != "" {
		ctx = provider.WithExecutionMode(ctx, provider.Capability(req.ExecutionMode))
	}
	if req.WorkingDirectory != "" {
		ctx = provider.WithWorkingDirectory(ctx, req.WorkingDirectory)
	}
	if req.OutputDirectory != "" {
		ctx = provider.WithOutputDirectory(ctx, req.OutputDirectory)
	}
	if req.ConflictStrategy != "" {
		ctx = provider.WithConflictStrategy(ctx, req.ConflictStrategy)
	}
	if req.Backup {
		ctx = provider.WithBackup(ctx, req.Backup)
	}
	var err error
	ctx, err = unmarshalIterationContext(ctx, req.IterationContext)
	if err != nil {
		return ctx, err
	}
	if len(req.Parameters) > 0 {
		var params map[string]any
		if err := json.Unmarshal(req.Parameters, &params); err != nil {
			return ctx, fmt.Errorf("failed to decode parameters: %w", err)
		}
		ctx = provider.WithParameters(ctx, params)
	}
	ctx = unmarshalSolutionMeta(ctx, req.SolutionMetadata)
	return ctx, nil
}

// ---- Stream forwarder ----

type streamForwarder struct {
	stream proto.PluginService_ExecuteProviderStreamServer
	mu     sync.Mutex
	err    error
}

func newStreamForwarder(stream proto.PluginService_ExecuteProviderStreamServer) *streamForwarder {
	return &streamForwarder{stream: stream}
}

func (f *streamForwarder) forward(chunk StreamChunk) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return
	}
	var protoChunk proto.ExecuteProviderStreamChunk
	switch {
	case chunk.Stdout != nil:
		protoChunk.Chunk = &proto.ExecuteProviderStreamChunk_Stdout{Stdout: chunk.Stdout}
	case chunk.Stderr != nil:
		protoChunk.Chunk = &proto.ExecuteProviderStreamChunk_Stderr{Stderr: chunk.Stderr}
	case chunk.Result != nil || chunk.Error != "":
		outputBytes, marshalErr := json.Marshal(chunk.Result)
		if marshalErr != nil {
			protoChunk.Chunk = &proto.ExecuteProviderStreamChunk_Result{
				Result: &proto.ExecuteProviderResponse{Error: fmt.Sprintf("failed to encode output: %v", marshalErr)},
			}
		} else {
			protoChunk.Chunk = &proto.ExecuteProviderStreamChunk_Result{
				Result: &proto.ExecuteProviderResponse{Output: outputBytes, Error: chunk.Error},
			}
		}
	default:
		return
	}
	if sErr := f.stream.Send(&protoChunk); sErr != nil {
		f.err = sErr
	}
}

// ---- Descriptor conversion ----

func schemaToProto(schema *jsonschema.Schema) *proto.Schema {
	if schema == nil || len(schema.Properties) == 0 {
		return nil
	}
	ps := &proto.Schema{Parameters: make(map[string]*proto.Parameter, len(schema.Properties))}
	requiredSet := make(map[string]bool, len(schema.Required))
	for _, name := range schema.Required {
		requiredSet[name] = true
	}
	for name, prop := range schema.Properties {
		ps.Parameters[name] = paramToProto(prop, requiredSet[name])
	}
	return ps
}

func paramToProto(prop *jsonschema.Schema, required bool) *proto.Parameter {
	var defaultValue []byte
	if prop.Default != nil {
		var err error
		defaultValue, err = json.Marshal(prop.Default)
		if err != nil {
			defaultValue = []byte(`"<marshal error>"`)
		}
	}
	exampleStr := ""
	if len(prop.Examples) > 0 {
		if b, err := json.Marshal(prop.Examples[0]); err == nil {
			exampleStr = string(b)
		}
	}
	var maxLen, minLen int32
	if prop.MaxLength != nil {
		maxLen = int32(*prop.MaxLength) //nolint:gosec
	}
	if prop.MinLength != nil {
		minLen = int32(*prop.MinLength) //nolint:gosec
	}
	var minItems, maxItems int32
	if prop.MinItems != nil {
		minItems = int32(*prop.MinItems) //nolint:gosec
	}
	if prop.MaxItems != nil {
		maxItems = int32(*prop.MaxItems) //nolint:gosec
	}
	p := &proto.Parameter{
		Type: prop.Type, Required: required, Description: prop.Description,
		DefaultValue: defaultValue, Example: exampleStr,
		MaxLength: maxLen, MinLength: minLen, Pattern: prop.Pattern, Format: prop.Format,
		MinItems: minItems, MaxItems: maxItems,
	}
	if prop.Minimum != nil {
		p.Minimum = *prop.Minimum
		p.HasMinimum = true
	}
	if prop.Maximum != nil {
		p.Maximum = *prop.Maximum
		p.HasMaximum = true
	}
	if prop.ExclusiveMinimum != nil {
		p.ExclusiveMinimum = *prop.ExclusiveMinimum
		p.HasExclusiveMinimum = true
	}
	if prop.ExclusiveMaximum != nil {
		p.ExclusiveMaximum = *prop.ExclusiveMaximum
		p.HasExclusiveMaximum = true
	}
	if len(prop.Enum) > 0 {
		p.EnumValues = make([][]byte, 0, len(prop.Enum))
		for _, v := range prop.Enum {
			if b, err := json.Marshal(v); err == nil {
				p.EnumValues = append(p.EnumValues, b)
			}
		}
	}
	return p
}

func descriptorToProto(desc *provider.Descriptor) *proto.ProviderDescriptor {
	version := ""
	if desc.Version != nil {
		version = desc.Version.String()
	}
	pd := &proto.ProviderDescriptor{
		Name: desc.Name, DisplayName: desc.DisplayName, Description: desc.Description,
		Version: version, Category: desc.Category, ApiVersion: desc.APIVersion,
		Capabilities:    make([]string, len(desc.Capabilities)),
		SensitiveFields: desc.SensitiveFields, Tags: desc.Tags, Icon: desc.Icon,
		Deprecated: desc.IsDeprecated, Beta: desc.Beta,
		HasExtractDependencies: desc.ExtractDependencies != nil,
	}
	for i, cap := range desc.Capabilities {
		pd.Capabilities[i] = string(cap)
	}
	for _, link := range desc.Links {
		pd.Links = append(pd.Links, &proto.Link{Name: link.Name, Url: link.URL})
	}
	for _, ex := range desc.Examples {
		pd.Examples = append(pd.Examples, &proto.Example{Name: ex.Name, Description: ex.Description, Yaml: ex.YAML})
	}
	for _, m := range desc.Maintainers {
		pd.Maintainers = append(pd.Maintainers, &proto.Contact{Name: m.Name, Email: m.Email})
	}
	pd.Schema = schemaToProto(desc.Schema)
	if desc.Schema != nil {
		if raw, err := json.Marshal(desc.Schema); err == nil {
			pd.RawSchema = raw
		}
	}
	if len(desc.OutputSchemas) > 0 {
		pd.OutputSchemas = make(map[string]*proto.Schema)
		pd.RawOutputSchemas = make(map[string][]byte, len(desc.OutputSchemas))
		for cap, schema := range desc.OutputSchemas {
			if schema == nil {
				continue
			}
			if raw, err := json.Marshal(schema); err == nil {
				pd.RawOutputSchemas[string(cap)] = raw
			}
			if ps := schemaToProto(schema); ps != nil {
				pd.OutputSchemas[string(cap)] = ps
			}
		}
	}
	return pd
}

// ProtoToDescriptor converts proto.ProviderDescriptor to provider.Descriptor.
func ProtoToDescriptor(pd *proto.ProviderDescriptor) (*provider.Descriptor, error) {
	var version *semver.Version
	if pd.Version != "" {
		var err error
		version, err = semver.NewVersion(pd.Version)
		if err != nil {
			return nil, fmt.Errorf("plugin %q has invalid semver version %q: %w", pd.Name, pd.Version, err)
		}
	}
	desc := &provider.Descriptor{
		Name: pd.Name, DisplayName: pd.DisplayName, Description: pd.Description,
		Version: version, Category: pd.Category, APIVersion: pd.ApiVersion,
		Capabilities:    make([]provider.Capability, len(pd.Capabilities)),
		SensitiveFields: pd.SensitiveFields, Tags: pd.Tags, Icon: pd.Icon,
		IsDeprecated: pd.Deprecated, Beta: pd.Beta,
	}
	for i, cap := range pd.Capabilities {
		desc.Capabilities[i] = provider.Capability(cap)
	}
	for _, link := range pd.Links {
		desc.Links = append(desc.Links, provider.Link{Name: link.Name, URL: link.Url})
	}
	for _, ex := range pd.Examples {
		desc.Examples = append(desc.Examples, provider.Example{Name: ex.Name, Description: ex.Description, YAML: ex.Yaml})
	}
	for _, m := range pd.Maintainers {
		desc.Maintainers = append(desc.Maintainers, provider.Contact{Name: m.Name, Email: m.Email})
	}
	desc.Schema = unmarshalSchemaOrFallback(pd.RawSchema, pd.Schema)
	desc.OutputSchemas = unmarshalOutputSchemas(pd.RawOutputSchemas, pd.OutputSchemas)
	if pd.HasExtractDependencies {
		desc.ExtractDependencies = func(_ map[string]any) []string { return nil }
	}
	return desc, nil
}

// ---- Proto -> Schema conversion ----

func protoSchemaToJSON(ps *proto.Schema) *jsonschema.Schema {
	schema := &jsonschema.Schema{Type: "object", Properties: make(map[string]*jsonschema.Schema)}
	var required []string
	for name, param := range ps.Parameters {
		schema.Properties[name] = protoParamToJSON(param)
		if param.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	schema.Required = required
	return schema
}

func unmarshalSchemaOrFallback(raw []byte, ps *proto.Schema) *jsonschema.Schema {
	if len(raw) > 0 {
		var schema jsonschema.Schema
		if err := json.Unmarshal(raw, &schema); err == nil {
			return &schema
		}
	}
	if ps != nil {
		return protoSchemaToJSON(ps)
	}
	return nil
}

func unmarshalOutputSchemas(rawSchemas map[string][]byte, protoSchemas map[string]*proto.Schema) map[provider.Capability]*jsonschema.Schema {
	if len(rawSchemas) > 0 {
		out := make(map[provider.Capability]*jsonschema.Schema, len(rawSchemas))
		for capStr, raw := range rawSchemas {
			var schema jsonschema.Schema
			if err := json.Unmarshal(raw, &schema); err == nil {
				out[provider.Capability(capStr)] = &schema
			}
		}
		return out
	}
	if len(protoSchemas) > 0 {
		out := make(map[provider.Capability]*jsonschema.Schema, len(protoSchemas))
		for capStr, ps := range protoSchemas {
			if ps == nil {
				continue
			}
			out[provider.Capability(capStr)] = protoSchemaToJSON(ps)
		}
		return out
	}
	return nil
}

func protoParamToJSON(param *proto.Parameter) *jsonschema.Schema {
	prop := &jsonschema.Schema{
		Type: param.Type, Description: param.Description, Pattern: param.Pattern, Format: param.Format,
	}
	if len(param.DefaultValue) > 0 {
		prop.Default = json.RawMessage(param.DefaultValue)
	}
	if param.Example != "" {
		var example any
		if err := json.Unmarshal([]byte(param.Example), &example); err == nil {
			prop.Examples = []any{example}
		}
	}
	if param.MaxLength > 0 {
		ml := int(param.MaxLength)
		prop.MaxLength = &ml
	}
	if param.MinLength > 0 {
		ml := int(param.MinLength)
		prop.MinLength = &ml
	}
	if param.HasMinimum {
		v := param.Minimum
		prop.Minimum = &v
	}
	if param.HasMaximum {
		v := param.Maximum
		prop.Maximum = &v
	}
	if param.HasExclusiveMinimum {
		v := param.ExclusiveMinimum
		prop.ExclusiveMinimum = &v
	}
	if param.HasExclusiveMaximum {
		v := param.ExclusiveMaximum
		prop.ExclusiveMaximum = &v
	}
	if param.MinItems > 0 {
		mi := int(param.MinItems)
		prop.MinItems = &mi
	}
	if param.MaxItems > 0 {
		mi := int(param.MaxItems)
		prop.MaxItems = &mi
	}
	if len(param.EnumValues) > 0 {
		prop.Enum = make([]any, 0, len(param.EnumValues))
		for _, raw := range param.EnumValues {
			var v any
			if err := json.Unmarshal(raw, &v); err == nil {
				prop.Enum = append(prop.Enum, v)
			}
		}
	}
	return prop
}
